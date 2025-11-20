// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package terraform

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform/internal/addrs"
	"github.com/hashicorp/terraform/internal/configs/configschema"
	"github.com/hashicorp/terraform/internal/plans"
	"github.com/hashicorp/terraform/internal/providers"
	testing_provider "github.com/hashicorp/terraform/internal/providers/testing"
	"github.com/hashicorp/terraform/internal/states"
	"github.com/zclconf/go-cty/cty"
)

// TestContext2Plan_stopBeforeStart tests that calling Stop() before
// a Plan operation starts will cause the Plan to abort early rather than
// proceeding with the operation.
//
// This is a regression test for issue #31371 where early SIGINT signals
// were not respected if a Terraform Core operation subsequently began.
func TestContext2Plan_stopBeforeStart(t *testing.T) {
	m := testModule(t, "apply-stop")

	p := &testing_provider.MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			ResourceTypes: map[string]providers.Schema{
				"indefinite": {
					Version: 1,
					Body: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"id": {
								Type:     cty.String,
								Computed: true,
							},
							"result": {
								Type:     cty.String,
								Computed: true,
							},
						},
					},
				},
			},
		},
	}

	planStarted := false
	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		// If we get here, it means the plan operation actually started,
		// which is what we're trying to prevent
		planStarted = true
		t.Error("Plan operation should not have started after Stop was called")

		return providers.PlanResourceChangeResponse{
			PlannedState: req.ProposedNewState,
		}
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.MustParseProviderSourceString("terraform.io/test/indefinite"): testProviderFuncFixed(p),
		},
	})

	// Call Stop before starting the Plan operation
	// This simulates receiving a SIGINT before the operation begins
	ctx.Stop()

	// Give Stop() a moment to complete
	time.Sleep(100 * time.Millisecond)

	// Now try to start the Plan operation
	// It should abort early and not actually run
	plan, diags := ctx.Plan(m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})

	// The plan should be nil or minimal since the operation was cancelled
	if plan != nil {
		// Check that the plan is empty or has no changes
		if len(plan.Changes.Resources) > 0 {
			t.Error("Expected plan to be empty or have no resource changes after early Stop")
		}
	}

	// We expect the operation to have been cancelled, so there might be errors
	// but the important thing is that planStarted remains false
	if planStarted {
		t.Fatal("Plan operation started despite Stop being called before it began")
	}

	// If there are diagnostics, they should indicate cancellation
	if diags.HasErrors() {
		t.Logf("Diagnostics (expected due to cancellation): %s", diags.Err())
	}
}

// TestContext2Apply_stopBeforeStart tests that calling Stop() before
// an Apply operation starts will cause the Apply to abort early.
func TestContext2Apply_stopBeforeStart(t *testing.T) {
	m := testModule(t, "apply-stop")

	p := &testing_provider.MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			ResourceTypes: map[string]providers.Schema{
				"indefinite": {
					Version: 1,
					Body: &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"id": {
								Type:     cty.String,
								Computed: true,
							},
							"result": {
								Type:     cty.String,
								Computed: true,
							},
						},
					},
				},
			},
		},
	}

	applyStarted := false
	p.ApplyResourceChangeFn = func(req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
		// If we get here, it means the apply operation actually started,
		// which is what we're trying to prevent
		applyStarted = true
		t.Error("Apply operation should not have started after Stop was called")

		return providers.ApplyResourceChangeResponse{
			NewState: cty.ObjectVal(map[string]cty.Value{
				"id":     cty.StringVal("test-id"),
				"result": cty.StringVal("test-result"),
			}),
		}
	}

	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
		return providers.PlanResourceChangeResponse{
			PlannedState: cty.ObjectVal(map[string]cty.Value{
				"id":     cty.UnknownVal(cty.String),
				"result": cty.UnknownVal(cty.String),
			}),
		}
	}

	ctx := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.MustParseProviderSourceString("terraform.io/test/indefinite"): testProviderFuncFixed(p),
		},
	})

	// First create a plan
	plan, diags := ctx.Plan(m, states.NewState(), &PlanOpts{
		Mode: plans.NormalMode,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected plan errors: %s", diags.Err())
	}

	// Create a new context for apply
	ctx2 := testContext2(t, &ContextOpts{
		Providers: map[addrs.Provider]providers.Factory{
			addrs.MustParseProviderSourceString("terraform.io/test/indefinite"): testProviderFuncFixed(p),
		},
	})

	// Call Stop before starting the Apply operation
	ctx2.Stop()

	// Give Stop() a moment to complete
	time.Sleep(100 * time.Millisecond)

	// Now try to start the Apply operation
	// It should abort early and not actually run
	_, applyDiags := ctx2.Apply(plan, m, nil)

	// We expect the operation to have been cancelled, so there might be errors
	// but the important thing is that applyStarted remains false
	if applyStarted {
		t.Fatal("Apply operation started despite Stop being called before it began")
	}

	// If there are diagnostics, log them
	if applyDiags.HasErrors() {
		t.Logf("Diagnostics (may be expected due to cancellation): %s", applyDiags.Err())
	}
}
