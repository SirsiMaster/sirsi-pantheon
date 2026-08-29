package dashboard

import "testing"

func TestSNEReadModelWithdrawsReadyFromUnsupportedActiveTuple(t *testing.T) {
	model := SNEReadModel{
		Ready:        true,
		ServiceState: "ready",
		ActiveModel:  "gemma-active",
		Catalog: []SNECatalogItem{{
			ModelID: "gemma-active", Active: true, State: "ready",
			SupportStatus: "unqualified", ActionLabel: "Stop", ActionKind: "stop", ActionEnabled: true,
		}},
	}

	enforceSNEReadModelSupportInvariant(&model)
	if model.Ready || model.ServiceState != "support-mismatch" || model.Recovery == "" {
		t.Fatalf("unsupported active tuple remained ready: %+v", model)
	}
	item := model.Catalog[0]
	if item.State != "support-mismatch" || item.ActionKind != "stop" || !item.ActionEnabled || item.Reason == "" {
		t.Fatalf("unsupported tuple lost safe recovery: %+v", item)
	}
}

func TestSNEReadModelPreservesReadyForReleaseSupportedActiveTuple(t *testing.T) {
	model := SNEReadModel{
		Ready:        true,
		ServiceState: "ready",
		ActiveModel:  "gemma-active",
		Catalog: []SNECatalogItem{{
			ModelID: "gemma-active", Active: true, State: "ready",
			SupportStatus: "release-supported", ActionLabel: "Stop", ActionKind: "stop", ActionEnabled: true,
		}},
	}

	enforceSNEReadModelSupportInvariant(&model)
	if !model.Ready || model.ServiceState != "ready" || model.Recovery != "" || model.Catalog[0].State != "ready" {
		t.Fatalf("release-supported tuple was altered: %+v", model)
	}
}

func TestSNEReadModelRejectsActiveModelIdentityDrift(t *testing.T) {
	model := SNEReadModel{
		Ready:        true,
		ServiceState: "ready",
		ActiveModel:  "gemma-expected",
		Catalog: []SNECatalogItem{{
			ModelID: "gemma-other", Active: true, State: "ready", SupportStatus: "release-supported",
		}},
	}

	enforceSNEReadModelSupportInvariant(&model)
	if model.Ready || model.ServiceState != "support-mismatch" {
		t.Fatalf("active model identity drift remained ready: %+v", model)
	}
}
