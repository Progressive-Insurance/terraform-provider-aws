// Code generated by internal/generate/servicepackages/main.go; DO NOT EDIT.

package devicefarm

import (
	"context"

	"github.com/hashicorp/terraform-provider-aws/internal/types"
	"github.com/hashicorp/terraform-provider-aws/names"
)

type servicePackage struct{}

func (p *servicePackage) FrameworkDataSources(ctx context.Context) []*types.ServicePackageFrameworkDataSource {
	return []*types.ServicePackageFrameworkDataSource{}
}

func (p *servicePackage) FrameworkResources(ctx context.Context) []*types.ServicePackageFrameworkResource {
	return []*types.ServicePackageFrameworkResource{}
}

func (p *servicePackage) SDKDataSources(ctx context.Context) []*types.ServicePackageSDKDataSource {
	return []*types.ServicePackageSDKDataSource{}
}

func (p *servicePackage) SDKResources(ctx context.Context) []*types.ServicePackageSDKResource {
	return []*types.ServicePackageSDKResource{
		{
			Factory:  ResourceDevicePool,
			TypeName: "aws_devicefarm_device_pool",
		},
		{
			Factory:  ResourceInstanceProfile,
			TypeName: "aws_devicefarm_instance_profile",
		},
		{
			Factory:  ResourceNetworkProfile,
			TypeName: "aws_devicefarm_network_profile",
		},
		{
			Factory:  ResourceProject,
			TypeName: "aws_devicefarm_project",
		},
		{
			Factory:  ResourceTestGridProject,
			TypeName: "aws_devicefarm_test_grid_project",
		},
		{
			Factory:  ResourceUpload,
			TypeName: "aws_devicefarm_upload",
		},
	}
}

func (p *servicePackage) ServicePackageName() string {
	return names.DeviceFarm
}

var ServicePackage = &servicePackage{}
