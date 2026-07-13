// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/juju/juju/api/client/action"
	"github.com/juju/juju/rpc/params"

	"github.com/juju/terraform-provider-juju/internal/juju"
	"github.com/juju/terraform-provider-juju/internal/wait"
)

var _ resource.Resource = &actionResource{}
var _ resource.ResourceWithConfigure = &actionResource{}

// NewActionResource returns a new action resource.
func NewActionResource() resource.Resource {
	return &actionResource{}
}

type actionResource struct {
	client *juju.Client

	// subCtx is the context created with the new tflog subsystem for actions.
	subCtx context.Context
}

// actionResourceModel is the model for the juju_action resource.
type actionResourceModel struct {
	// ModelUUID is the UUID of the model where the action will be run.
	ModelUUID types.String `tfsdk:"model_uuid"`
	// ApplicationName is the name of the application to run the action on.
	ApplicationName types.String `tfsdk:"application_name"`
	// ActionName is the name of the action to run.
	ActionName types.String `tfsdk:"action_name"`
	// Unit is the unit name (e.g. "ubuntu/0" or "ubuntu/leader") to run
	// the action on.
	Unit types.String `tfsdk:"unit"`
	// Args are the arguments to pass to the action.
	Args types.Map `tfsdk:"args"`
	// ActionID is the ID of the enqueued action. It is computed after the
	// action has been enqueued.
	ActionID types.String `tfsdk:"action_id"`
	// Output is the output of the action as a JSON string. The consumer
	// can use jsondecode() to extract values from it.
	Output types.String `tfsdk:"output"`
	// OutputMap is the output of the action as a dynamic map, mirroring the
	// structure returned by Juju. Nested values are preserved, so consumers
	// can index into it directly without calling jsondecode().
	OutputMap types.Dynamic `tfsdk:"output_map"`
	// ID required by the testing framework.
	ID types.String `tfsdk:"id"`
}

// Metadata returns the full resource name as used in terraform plans.
func (r *actionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_action"
}

// Schema returns the schema for the action resource.
func (r *actionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A resource that represents a Juju action. The action is run and its result awaited during the resource's creation. " +
			"The action's output is set as a computed field that can be used by other resources after the resource has been created.",
		Attributes: map[string]schema.Attribute{
			"model_uuid": schema.StringAttribute{
				Description: "The UUID of the model where the action will be run. Changing this value will cause the resource to be destroyed and recreated.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"application_name": schema.StringAttribute{
				Description: "The name of the application to run the action on. Changing this value will cause the resource to be destroyed and recreated.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"action_name": schema.StringAttribute{
				Description: "The name of the action to run. Changing this value will cause the resource to be destroyed and recreated.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"unit": schema.StringAttribute{
				Description: "The unit name (e.g. \"ubuntu/0\" or \"ubuntu/leader\") to run the action on. Changing this value will cause the resource to be destroyed and recreated.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"args": schema.MapAttribute{
				Description: "The arguments to pass to the action. Changing this value will cause the resource to be destroyed and recreated.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"action_id": schema.StringAttribute{
				Description: "The ID of the enqueued action.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"output": schema.StringAttribute{
				Description: "The output of the action as a JSON string. Use jsondecode() to extract values from it.",
				Computed:    true,
			},
			"output_map": schema.DynamicAttribute{
				Description: "The output of the action as a dynamic map, mirroring the structure returned by Juju. Nested values are preserved, so it can be indexed directly.",
				Computed:    true,
			},
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure enables provider-level data or clients to be set in the
// resource.
func (r *actionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	provider, diags := getProviderData(req, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = provider.Client
	r.subCtx = tflog.NewSubsystem(ctx, LogResourceAction)
}

// Create enqueues the action and waits for its completion.
func (r *actionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "action", "create")
		return
	}

	var plan actionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	modelUUID := plan.ModelUUID.ValueString()
	appName := plan.ApplicationName.ValueString()
	actionName := plan.ActionName.ValueString()
	receiver := plan.Unit.ValueString()

	// If the receiver targets the leader unit (e.g. "ubuntu/leader"),
	// resolve it to a concrete unit name before enqueuing the action.
	// The resolved name is only used for the API call; the state keeps
	// the user's original value (e.g. "ubuntu/leader").
	if juju.IsLeaderReceiver(receiver) {
		var err error
		receiver, err = r.client.Actions.ResolveLeaderUnit(ctx, juju.ResolveLeaderUnitArgs{
			ModelUUID:       modelUUID,
			ApplicationName: appName,
		})
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to resolve leader unit for application %q: %s", appName, err))
			return
		}
	}

	// Build the action parameters from the args map.
	actionParams := make(map[string]interface{})
	if !plan.Args.IsNull() && !plan.Args.IsUnknown() {
		argsMap := make(map[string]string, len(plan.Args.Elements()))
		resp.Diagnostics.Append(plan.Args.ElementsAs(ctx, &argsMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range argsMap {
			actionParams[k] = v
		}
	}

	// Verify that the action exists on the charm by querying CharmHub.
	// This is done once, before retrying the enqueue, to avoid retrying
	// forever when the action genuinely doesn't exist.
	actionExists, err := r.client.Applications.ActionExists(ctx, modelUUID, appName, actionName)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to verify action %q on CharmHub: %s", actionName, err))
		return
	}
	if !actionExists {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Action %q is not defined on the charm deployed for application %q", actionName, appName))
		return
	}

	// Enqueue the action. The unit's charm may not be fully installed
	// yet, which would cause the enqueue to fail with "no actions
	// defined on charm". We retry until the charm is installed. Since
	// we already verified the action exists on CharmHub, we know the
	// error is transient.
	actionID, err := waitEnqueueAction(ctx, r, modelUUID, receiver, actionName, actionParams)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to enqueue action %q: %s", actionName, err))
		return
	}
	plan.ActionID = types.StringValue(actionID)
	plan.ID = types.StringValue(newActionResourceID(modelUUID, appName, actionName, actionID))

	// Save the state with the action_id before waiting for the result.
	// This way, if the wait fails and the resource is tainted, the
	// action_id is already in state and a subsequent apply's Read can
	// wait for the existing action's result instead of re-enqueuing.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Wait for the action to complete and populate the output.
	actionResult, err := waitForActionResult(ctx, r.client, r.actionLogf(), modelUUID, actionID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to wait for action %q to complete: %s", actionName, err))
		return
	}

	// Set the output.
	plan.Output, err = actionResultToOutput(actionResult)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to convert action output: %s", err))
		return
	}
	plan.OutputMap, err = actionResultToOutputMap(ctx, actionResult)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to convert action output map: %s", err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read reads the action result and updates the state.
func (r *actionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		addClientNotConfiguredError(&resp.Diagnostics, "action", "read")
		return
	}

	var state actionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the output is not yet populated, the action may still be running,
	// this can happen when the wait fails after the action has been enqueued.
	// Wait for the action to complete and populate the output.
	if state.Output.IsNull() || state.Output.IsUnknown() {
		actionResult, err := waitForActionResult(ctx, r.client, r.actionLogf(), state.ModelUUID.ValueString(), state.ActionID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to wait for action %q to complete: %s", state.ActionName.ValueString(), err))
			return
		}
		state.Output, err = actionResultToOutput(actionResult)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to convert action output: %s", err))
			return
		}
		state.OutputMap, err = actionResultToOutputMap(ctx, actionResult)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to convert action output map: %s", err))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op for the action resource. All schema attributes use
// RequiresReplace, so any change causes the resource to be destroyed and
// recreated rather than updated.
func (r *actionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// This should never happen, let's return an error in case it does.
	resp.Diagnostics.AddError("Provider Error", "Update called on action resource, but all attributes are RequiresReplace. This should never happen.")
}

// Delete is a no-op for the action resource. Actions cannot be deleted
// from Juju, so we just remove the resource from the Terraform state.
func (r *actionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

// actionLogf returns a logging function bound to the resource's action
// logging subsystem, suitable for passing to wait.WaitFor.
func (r *actionResource) actionLogf() wait.LogFunc {
	return func(msg string, additionalFields ...map[string]interface{}) {
		tflog.SubsystemDebug(r.subCtx, LogResourceAction, msg, additionalFields...)
	}
}

// assertActionCompleted asserts that the action has completed successfully.
// If the action is still running or pending, it returns a retry error.
// If the action failed, was cancelled, or errored, it returns a fatal error.
func assertActionCompleted(resultFromAPI action.ActionResult) error {
	if resultFromAPI.Error != nil {
		return resultFromAPI.Error
	}
	switch resultFromAPI.Status {
	case params.ActionRunning, params.ActionPending:
		return juju.NewRetryReadError("action is still running or pending, waiting for completion")
	case params.ActionCompleted:
		return nil
	default:
		return errors.New("action did not complete successfully, status: " + resultFromAPI.Status)
	}
}

// waitEnqueueAction retries enqueuing an action until it succeeds. The unit's
// charm may not be fully installed yet, which would cause the enqueue to fail
// with a NoActionsDefinedError. Only that specific error is retried; all
// other enqueue errors are fatal.
func waitEnqueueAction(ctx context.Context, r *actionResource, modelUUID, receiver, actionName string, params map[string]interface{}) (string, error) {
	var actionID string
	_, err := wait.WaitFor(wait.WaitForCfg[juju.EnqueueActionArgs, string]{
		Context: ctx,
		Input: juju.EnqueueActionArgs{
			ModelUUID:  modelUUID,
			Receiver:   receiver,
			Name:       actionName,
			Parameters: params,
		},
		GetData: func(ctx context.Context, args juju.EnqueueActionArgs) (string, error) {
			id, err := r.client.Actions.EnqueueAction(ctx, args)
			if err != nil {
				return "", err
			}
			actionID = id
			return id, nil
		},
		NonFatalErrors: []error{juju.NoActionsDefinedError},
		Logf: func(msg string, additionalFields ...map[string]interface{}) {
			tflog.SubsystemDebug(r.subCtx, LogResourceAction, msg, additionalFields...)
		},
	})
	return actionID, err
}

// waitForActionResult waits for the action identified by actionID to complete
// and returns its result. It is shared by the action resource and the action
// data source.
func waitForActionResult(ctx context.Context, client *juju.Client, logf wait.LogFunc, modelUUID, actionID string) (action.ActionResult, error) {
	return wait.WaitFor(wait.WaitForCfg[juju.ActionResultArgs, action.ActionResult]{
		Context: ctx,
		Input: juju.ActionResultArgs{
			ModelUUID: modelUUID,
			ActionID:  actionID,
		},
		GetData:        client.Actions.ActionResult,
		DataAssertions: []wait.Assert[action.ActionResult]{assertActionCompleted},
		NonFatalErrors: []error{juju.RetryReadError},
		Logf:           logf,
	})
}

// actionResultToOutput converts an action result's output into a JSON
// string suitable for storing in Terraform state. The consumer can use
// jsondecode() to extract values from it.
func actionResultToOutput(actionResult action.ActionResult) (types.String, error) {
	if len(actionResult.Output) == 0 {
		return types.StringNull(), nil
	}
	b, err := json.Marshal(actionResult.Output)
	if err != nil {
		return types.StringNull(), fmt.Errorf("unable to marshal action output: %w", err)
	}
	return types.StringValue(string(b)), nil
}

// actionResultToOutputMap converts an action result's output into a dynamic
// value that mirrors the structure returned by Juju. Nested maps, lists and
// scalars are preserved, so consumers can index into the value directly.
func actionResultToOutputMap(ctx context.Context, actionResult action.ActionResult) (types.Dynamic, error) {
	if len(actionResult.Output) == 0 {
		return types.DynamicNull(), nil
	}
	// Round-trip through JSON so that all values are normalised to the set
	// of types understood by goValueToTerraform (map, slice, string, bool
	// and json.Number).
	b, err := json.Marshal(actionResult.Output)
	if err != nil {
		return types.DynamicNull(), fmt.Errorf("unable to marshal action output: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return types.DynamicNull(), fmt.Errorf("unable to decode action output: %w", err)
	}

	tfType := goValueToTerraformType(decoded)
	tfValue, err := goValueToTerraform(decoded)
	if err != nil {
		return types.DynamicNull(), err
	}
	attrValue, err := types.DynamicType.ValueFromTerraform(ctx, tftypes.NewValue(tfType, tfValue))
	if err != nil {
		return types.DynamicNull(), fmt.Errorf("unable to build dynamic action output: %w", err)
	}
	dynamic, ok := attrValue.(types.Dynamic)
	if !ok {
		return types.DynamicNull(), fmt.Errorf("unexpected action output value type %T", attrValue)
	}
	return dynamic, nil
}

// goValueToTerraformType returns the tftypes.Type describing the given Go
// value, which must be one of the types produced by encoding/json with
// UseNumber: map[string]any, []any, string, bool, json.Number or nil.
func goValueToTerraformType(v any) tftypes.Type {
	switch val := v.(type) {
	case map[string]any:
		attrTypes := make(map[string]tftypes.Type, len(val))
		for k, elem := range val {
			attrTypes[k] = goValueToTerraformType(elem)
		}
		return tftypes.Object{AttributeTypes: attrTypes}
	case []any:
		elemTypes := make([]tftypes.Type, len(val))
		for i, elem := range val {
			elemTypes[i] = goValueToTerraformType(elem)
		}
		return tftypes.Tuple{ElementTypes: elemTypes}
	case bool:
		return tftypes.Bool
	case json.Number:
		return tftypes.Number
	case string:
		return tftypes.String
	default:
		return tftypes.DynamicPseudoType
	}
}

// goValueToTerraform converts a Go value produced by encoding/json with
// UseNumber into the corresponding tftypes representation.
func goValueToTerraform(v any) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]tftypes.Value, len(val))
		for k, elem := range val {
			tfElem, err := goValueToTerraform(elem)
			if err != nil {
				return nil, err
			}
			out[k] = tftypes.NewValue(goValueToTerraformType(elem), tfElem)
		}
		return out, nil
	case []any:
		out := make([]tftypes.Value, len(val))
		for i, elem := range val {
			tfElem, err := goValueToTerraform(elem)
			if err != nil {
				return nil, err
			}
			out[i] = tftypes.NewValue(goValueToTerraformType(elem), tfElem)
		}
		return out, nil
	case bool:
		return val, nil
	case json.Number:
		f, ok := new(big.Float).SetString(val.String())
		if !ok {
			return nil, fmt.Errorf("unable to parse number %q in action output", val.String())
		}
		return f, nil
	case string:
		return val, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported action output value type %T", v)
	}
}

// newActionResourceID builds the resource ID from its components.
// The ID is composed of the model UUID, application name, action name and
// the enqueued action ID, separated by ":".
func newActionResourceID(modelUUID, appName, actionName, actionID string) string {
	return fmt.Sprintf("%s:%s:%s:%s", modelUUID, appName, actionName, actionID)
}
