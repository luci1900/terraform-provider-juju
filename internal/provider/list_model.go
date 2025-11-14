package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/juju/terraform-provider-juju/internal/juju"
)

type modelLister struct {
	client *juju.Client
	config juju.Config

	// context for the logging subsystem.
	subCtx context.Context
}

func NewModelLister() list.ListResourceWithConfigure {
	return &modelLister{}
}

func (r *modelLister) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	provider, ok := req.ProviderData.(juju.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected juju.ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = provider.Client
	r.config = provider.Config
	r.subCtx = tflog.NewSubsystem(ctx, LogResourceModel)
}

func (r *modelLister) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_model"
}

func (r *modelLister) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Attributes: map[string]listschema.Attribute{},
	}
}

func (r *modelLister) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	item := &modelResourceModel{
		ID:   types.StringValue("my_model"),
		Name: types.StringValue("My Model"),
	}

	//state := &tfsdk.State{}

	stream.Results = func(push func(list.ListResult) bool) {
		// Initialize a new result object for each thing
		result := req.NewListResult(ctx)

		// Set the user-friendly name of this thing
		result.DisplayName = item.Name.String()

		// Set resource identity data on the result
		result.Diagnostics.Append(result.Identity.Set(ctx, item.ID)...)
		if result.Diagnostics.HasError() {
			return
		}

		// Set the resource information on the result
		result.Diagnostics.Append(result.Resource.Set(ctx, item)...)
		if result.Diagnostics.HasError() {
			return
		}

		// Send the result to the stream.
		if !push(result) {
			return
		}
	}
}
