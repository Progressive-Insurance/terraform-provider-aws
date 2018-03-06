package aws

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsKmsGrant() *schema.Resource {
	return &schema.Resource{
		// There is no API for updating/modifying grants, hence no Update
		// Instead changes to most fields will force a new resource
		Create: resourceAwsKmsGrantCreate,
		Read:   resourceAwsKmsGrantRead,
		Delete: resourceAwsKmsGrantDelete,
		Exists: resourceAwsKmsGrantExists,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validateAwsKmsGrantName,
			},
			"key_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"grantee_principal": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateArn,
			},
			"operations": {
				Type: schema.TypeSet,
				Set:  schema.HashString,
				Elem: &schema.Schema{
					Type:         schema.TypeString,
					ValidateFunc: validateAwsKmsGrantOperation,
				},
				Required: true,
				ForceNew: true,
			},
			"constraints": {
				Type:     schema.TypeSet,
				Set:      resourceKmsGrantConstraintsHash,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"encryption_context_equals": {
							Type:     schema.TypeMap,
							Optional: true,
							ForceNew: true,
							Elem:     schema.TypeString,
							// ConflictsWith encryption_context_subset handled in Create, see kmsGrantConstraintsIsValid
						},
						"encryption_context_subset": {
							Type:     schema.TypeMap,
							Optional: true,
							ForceNew: true,
							Elem:     schema.TypeString,
							// ConflictsWith encryption_context_equals handled in Create, see kmsGrantConstraintsIsValid
						},
					},
				},
			},
			"retiring_principal": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validateArn,
			},
			"grant_creation_tokens": {
				Type:     schema.TypeSet,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
				ForceNew: true,
			},
			"grant_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"grant_token": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAwsKmsGrantCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).kmsconn

	input := kms.CreateGrantInput{
		GranteePrincipal: aws.String(d.Get("grantee_principal").(string)),
		KeyId:            aws.String(d.Get("key_id").(string)),
		Operations:       expandStringSet(d.Get("operations").(*schema.Set)),
	}

	if v, ok := d.GetOk("name"); ok {
		input.Name = aws.String(v.(string))
	}
	if v, ok := d.GetOk("constraints"); ok {
		if !kmsGrantConstraintsIsValid(v.(*schema.Set)) {
			return fmt.Errorf("[ERROR] A grant constraint can't have both encryption_context_equals and encryption_context_subset set")
		}
		input.Constraints = expandKmsGrantConstraints(v.(*schema.Set))
	}
	if v, ok := d.GetOk("retiring_principal"); ok {
		input.RetiringPrincipal = aws.String(v.(string))
	}
	if v, ok := d.GetOk("grant_creation_tokens"); ok {
		input.GrantTokens = expandStringSet(v.(*schema.Set))
	}

	log.Printf("[DEBUG]: Adding new KMS Grant: %s", input)

	var out *kms.CreateGrantOutput

	err := resource.Retry(3*time.Minute, func() *resource.RetryError {
		var err error

		out, err = conn.CreateGrant(&input)

		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				// Error Codes: https://docs.aws.amazon.com/sdk-for-go/api/service/kms/#KMS.CreateGrant
				// Under some circumstances a newly created IAM Role doesn't show up and causes
				// an InvalidArnException to be thrown.
				if awsErr.Code() == "DependencyTimeoutException" ||
					awsErr.Code() == "InternalException" ||
					awsErr.Code() == "InvalidArnException" {
					return resource.RetryableError(
						fmt.Errorf("[WARN] Error adding new KMS Grant for key: %s, retrying %s",
							*input.KeyId, err))
				}
			}
			log.Printf("[ERROR] An error occured creating new AWS KMS Grant: %s", err)
			return resource.NonRetryableError(err)
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Created new KMS Grant: %s", *out.GrantId)
	d.SetId(*out.GrantId)
	d.Set("grant_id", out.GrantId)
	d.Set("grant_token", out.GrantToken)

	return resourceAwsKmsGrantRead(d, meta)
}

func resourceAwsKmsGrantRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).kmsconn

	grantId := d.Id()
	keyId := d.Get("key_id").(string)

	log.Printf("[DEBUG] Looking for grant id: %s", grantId)
	grant, err := findKmsGrantByIdWithRetry(conn, keyId, grantId)

	if err != nil {
		return err
	}

	if grant == nil {
		log.Printf("[WARN] %s KMS grant id not found for key id %s, removing from state file", grantId, keyId)
		d.SetId("")
	}

	if grant != nil {
		// The grant sometimes contains principals that identified by their unique id: "AROAJYCVIVUZIMTXXXXX"
		// instead of "arn:aws:...", in this case don't update the state file
		if strings.HasPrefix(*grant.GranteePrincipal, "arn:aws") {
			d.Set("grantee_principal", grant.GranteePrincipal)
		} else {
			log.Printf(
				"[WARN] Unable to update grantee principal state %s for grant id %s for key id %s.",
				*grant.GranteePrincipal, grantId, keyId)
		}

		if grant.RetiringPrincipal != nil {
			if strings.HasPrefix(*grant.RetiringPrincipal, "arn:aws") {
				d.Set("retiring_principal", grant.RetiringPrincipal)
			} else {
				log.Printf(
					"[WARN] Unable to update retiring principal state %s for grant id %s for key id %s",
					*grant.RetiringPrincipal, grantId, keyId)
			}
		}

		if err := d.Set("operations", aws.StringValueSlice(grant.Operations)); err != nil {
			log.Printf("[DEBUG] Error setting operations for grant %s with error %s", grantId, err)
		}
		if *grant.Name != "" {
			d.Set("name", grant.Name)
		}
		if grant.Constraints != nil {
			if err := d.Set("constraints", flattenKmsGrantConstraints(grant.Constraints)); err != nil {
				log.Printf("[DEBUG] Error setting constraints for grant %s with error %s", grantId, err)
			}
		}
	}

	return nil
}

// Retiring grants requires special permissions (i.e. the
// caller to be root, retiree principal, or grantee principal with retire grant
// privileges). So just revoke grants.
func resourceAwsKmsGrantDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).kmsconn

	grantId := d.Get("grant_id").(string)
	keyId := d.Get("key_id").(string)
	input := kms.RevokeGrantInput{
		GrantId: aws.String(grantId),
		KeyId:   aws.String(keyId),
	}

	log.Printf("[DEBUG] Revoking KMS grant: %s", grantId)
	_, err := conn.RevokeGrant(&input)

	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Checking if grant is revoked: %s", grantId)
	err = waitForKmsGrantToBeRevoked(conn, keyId, grantId)

	if err != nil {
		return err
	}

	d.SetId("")
	return nil
}

func resourceAwsKmsGrantExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	conn := meta.(*AWSClient).kmsconn

	grantId := d.Id()
	keyId := d.Get("key_id").(string)

	log.Printf("[DEBUG] Looking for Grant: %s", grantId)
	grant, err := findKmsGrantByIdWithRetry(conn, keyId, grantId)

	if err != nil {
		return true, err
	}
	if grant != nil {
		return true, err
	}

	return false, nil
}

func getKmsGrantById(grants []*kms.GrantListEntry, grantIdentifier string) *kms.GrantListEntry {
	for _, grant := range grants {
		if *grant.GrantId == grantIdentifier {
			return grant
		}
	}

	return nil
}

/*
In the functions below it is not possible to use retryOnAwsCodes function, as there
is no describe grants call, so an error has to be created if the grant is or isn't returned
by the list grants call when expected.
*/

// NB: This function only retries the grant not being returned and some edge cases, while AWS Errors
// are handled by the findKmsGrantById function
func findKmsGrantByIdWithRetry(conn *kms.KMS, keyId string, grantId string) (*kms.GrantListEntry, error) {
	var grant *kms.GrantListEntry
	err := resource.Retry(3*time.Minute, func() *resource.RetryError {
		var err error
		grant, err = findKmsGrantById(conn, keyId, grantId, nil)

		if err != nil {
			if serr, ok := err.(KmsGrantMissingError); ok {
				// Force a retry if the grant should exist
				return resource.RetryableError(serr)
			}

			return resource.NonRetryableError(err)
		}

		return nil
	})

	return grant, err
}

// Used by the tests as well
func waitForKmsGrantToBeRevoked(conn *kms.KMS, keyId string, grantId string) error {
	err := resource.Retry(3*time.Minute, func() *resource.RetryError {
		grant, err := findKmsGrantById(conn, keyId, grantId, nil)
		if err != nil {
			if _, ok := err.(KmsGrantMissingError); ok {
				return nil
			}
		}

		if grant != nil {
			// Force a retry if the grant still exists
			return resource.RetryableError(
				fmt.Errorf("[DEBUG] Grant still exists while expected to be revoked, retyring revocation check: %s", *grant.GrantId))
		}

		return resource.NonRetryableError(err)
	})

	return err
}

// The ListGrants API defaults to listing only 50 grants
// Use a marker to iterate over all grants in "pages"
// NB: This function only retries on AWS Errors
func findKmsGrantById(conn *kms.KMS, keyId string, grantId string, marker *string) (*kms.GrantListEntry, error) {

	input := kms.ListGrantsInput{
		KeyId:  aws.String(keyId),
		Limit:  aws.Int64(int64(100)),
		Marker: marker,
	}

	var out *kms.ListGrantsResponse
	var err error
	var grant *kms.GrantListEntry

	err = resource.Retry(3*time.Minute, func() *resource.RetryError {
		out, err = conn.ListGrants(&input)

		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() == "NotFoundException" ||
					awsErr.Code() == "DependencyTimeoutException" ||
					awsErr.Code() == "InternalException" {
					return resource.RetryableError(err)
				}
			}
			return resource.NonRetryableError(err)
		}

		return nil
	})

	grant = getKmsGrantById(out.Grants, grantId)
	if grant != nil {
		return grant, nil
	}
	if *out.Truncated {
		log.Printf("[DEBUG] KMS Grant list truncated, getting next page via marker: %s", *out.NextMarker)
		return findKmsGrantById(conn, keyId, grantId, out.NextMarker)
	}

	return nil, NewKmsGrantMissingError(fmt.Sprintf("[DEBUG] Grant %s not found for key id: %s", grantId, keyId))
}

// Can't have both constraint options set:
// ValidationException: More than one constraint supplied
// NB: set.List() returns an empty map if the constraint is not set, filter those out
// using len(v) > 0
func kmsGrantConstraintsIsValid(constraints *schema.Set) bool {
	constraintCount := 0
	for _, raw := range constraints.List() {
		data := raw.(map[string]interface{})
		if v, ok := data["encryption_context_equals"].(map[string]interface{}); ok {
			if len(v) > 0 {
				constraintCount += 1
			}
		}
		if v, ok := data["encryption_context_subset"].(map[string]interface{}); ok {
			if len(v) > 0 {
				constraintCount += 1
			}
		}
	}

	if constraintCount > 1 {
		return false
	}
	return true
}

func expandKmsGrantConstraints(configured *schema.Set) *kms.GrantConstraints {
	if len(configured.List()) < 1 {
		return nil
	}

	var constraint kms.GrantConstraints

	for _, raw := range configured.List() {
		data := raw.(map[string]interface{})
		if contextEq, ok := data["encryption_context_equals"]; ok {
			constraint.SetEncryptionContextEquals(stringMapToPointers(contextEq.(map[string]interface{})))
		}
		if contextSub, ok := data["encryption_context_subset"]; ok {
			constraint.SetEncryptionContextSubset(stringMapToPointers(contextSub.(map[string]interface{})))
		}
	}

	return &constraint
}

func sortStringMapKeys(m map[string]*string) []string {
	keys := make([]string, 0)
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

// NB: For the constraint hash to be deterministic the order in which
// print the keys and values of the encryption context maps needs to be
// determistic, so sort them.
func sortedConcatStringMap(m map[string]*string, sep string) string {
	var strList []string
	mapKeys := sortStringMapKeys(m)
	for _, key := range mapKeys {
		strList = append(strList, key, *m[key])
	}
	return strings.Join(strList, sep)
}

// The hash needs to encapsulate what type of constraint it is
// as well as the keys and values of the constraint.
func resourceKmsGrantConstraintsHash(v interface{}) int {
	var buf bytes.Buffer
	m, castOk := v.(map[string]interface{})
	if !castOk {
		return 0
	}

	if v, ok := m["encryption_context_equals"]; ok {
		if len(v.(map[string]interface{})) > 0 {
			buf.WriteString(fmt.Sprintf("encryption_context_equals-%s-", sortedConcatStringMap(stringMapToPointers(v.(map[string]interface{})), "-")))
		}
	}
	if v, ok := m["encryption_context_subset"]; ok {
		if len(v.(map[string]interface{})) > 0 {
			buf.WriteString(fmt.Sprintf("encryption_context_subset-%s-", sortedConcatStringMap(stringMapToPointers(v.(map[string]interface{})), "-")))
		}
	}

	return hashcode.String(buf.String())
}

func flattenKmsGrantConstraints(constraint *kms.GrantConstraints) *schema.Set {
	constraints := schema.NewSet(resourceKmsGrantConstraintsHash, []interface{}{})
	if constraint == nil {
		return constraints
	}

	m := make(map[string]interface{}, 0)
	if constraint.EncryptionContextEquals != nil {
		if len(constraint.EncryptionContextEquals) > 0 {
			m["encryption_context_equals"] = pointersMapToStringList(constraint.EncryptionContextEquals)
		}
	}
	if constraint.EncryptionContextSubset != nil {
		if len(constraint.EncryptionContextSubset) > 0 {
			m["encryption_context_subset"] = pointersMapToStringList(constraint.EncryptionContextSubset)
		}
	}
	constraints.Add(m)

	return constraints
}

// Custom error, so we don't have to rely on
// the content of an error message
type KmsGrantMissingError string

func (e KmsGrantMissingError) Error() string {
	return e.Error()
}

func NewKmsGrantMissingError(msg string) KmsGrantMissingError {
	return KmsGrantMissingError(msg)
}
