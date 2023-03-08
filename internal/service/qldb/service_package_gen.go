// Code generated by internal/generate/servicepackages/main.go; DO NOT EDIT.

package qldb

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
	return []*types.ServicePackageSDKDataSource{
		{
			Factory:  DataSourceLedger,
			TypeName: "aws_qldb_ledger",
		},
	}
}

func (p *servicePackage) SDKResources(ctx context.Context) []*types.ServicePackageSDKResource {
	return []*types.ServicePackageSDKResource{
		{
			Factory:  ResourceLedger,
			TypeName: "aws_qldb_ledger",
		},
		{
			Factory:  ResourceStream,
			TypeName: "aws_qldb_stream",
		},
	}
}

func (p *servicePackage) ServicePackageName() string {
	return names.QLDB
}

var ServicePackage = &servicePackage{}
