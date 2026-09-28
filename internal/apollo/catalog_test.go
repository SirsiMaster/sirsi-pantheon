package apollo

import "testing"

func TestBuildPlanAcceptsConfiguredLocalEngineAndDetectedEstates(t *testing.T) {
	catalog := Catalog{
		Machine: Machine{ID: "this-mac", CPUCores: 10, MemoryBytes: 32 * gib},
		Engines: []Engine{{ID: "apollo-local-sne", State: "configured", ResidentModel: "resident-model"}},
		Estates: []ChipEstate{{ID: "cpu", Available: true}, {ID: "unified-memory", Available: true}, {ID: "gpu", Available: true}},
	}
	plan, err := BuildPlan(catalog, "apollo-local-sne", 6, 16*gib, 0, []string{"cpu", "gpu"})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.Execution != "planned; SNE admission is required before inference starts" {
		t.Fatalf("execution = %q", plan.Execution)
	}
}

func TestBuildPlanRetainsUnqualifiedOrRejectsDuplicateEstate(t *testing.T) {
	catalog := Catalog{
		Machine: Machine{ID: "this-mac", CPUCores: 8, MemoryBytes: 16 * gib},
		Engines: []Engine{{ID: "apollo-local-sne", State: "configured"}},
		Estates: []ChipEstate{{ID: "cpu", Available: true}, {ID: "gpu", Available: false}},
	}
	plan, err := BuildPlan(catalog, "apollo-local-sne", 4, 8*gib, 0, []string{"gpu"})
	if err != nil {
		t.Fatalf("BuildPlan() rejected an enumerated unqualified estate: %v", err)
	}
	if got, want := plan.UnavailableEstates, []string{"gpu"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("unavailable estates = %#v, want %#v", got, want)
	}
	if plan.Execution != "planned with requested estates awaiting SNE qualification; inference cannot start until SNE admits them" {
		t.Fatalf("execution = %q", plan.Execution)
	}
	if _, err := BuildPlan(catalog, "apollo-local-sne", 4, 8*gib, 0, []string{"cpu", "cpu"}); err == nil {
		t.Fatal("BuildPlan() accepted a duplicate estate")
	}
}

func TestBuildPlanRejectsEstateOutsideSelectedMachineReceipt(t *testing.T) {
	catalog := Catalog{
		Machine: Machine{ID: "this-mac", Name: "This Mac", CPUCores: 8, MemoryBytes: 16 * gib, ChipEstates: []string{"cpu"}},
		Engines: []Engine{{ID: "apollo-local-sne", State: "configured"}},
		Estates: []ChipEstate{{ID: "cpu", Available: true}, {ID: "gpu", Available: false}},
	}
	if _, err := BuildPlan(catalog, "apollo-local-sne", 4, 8*gib, 0, []string{"gpu"}); err == nil {
		t.Fatal("BuildPlan() accepted an estate outside the selected machine receipt")
	}
}

func TestEstateIDsRetainsDetectedUnqualifiedEstate(t *testing.T) {
	got := estateIDs([]ChipEstate{
		{ID: "cpu", Available: true},
		{ID: "neural-engine", Available: false},
	})
	if len(got) != 2 || got[0] != "cpu" || got[1] != "neural-engine" {
		t.Fatalf("estateIDs() = %#v, want detected estates including unqualified neural engine", got)
	}
}

func TestBuildPlanForMachineRejectsAnUnmeasuredMachine(t *testing.T) {
	catalog := Catalog{
		Machine:  Machine{ID: "this-mac", CPUCores: 8, MemoryBytes: 16 * gib},
		Machines: []Machine{{ID: "this-mac", CPUCores: 8, MemoryBytes: 16 * gib}},
		Engines:  []Engine{{ID: "apollo-local-sne", State: "configured"}},
		Estates:  []ChipEstate{{ID: "cpu", Available: true}},
	}
	if _, err := BuildPlanForMachine(catalog, "peer-without-receipt", "apollo-local-sne", 4, 8*gib, 0, []string{"cpu"}); err == nil {
		t.Fatal("BuildPlanForMachine() accepted an unmeasured machine")
	}
}

func TestBuildPlanForMachineBindsRouteAndEstatesToSelectedReceipt(t *testing.T) {
	catalog := Catalog{
		Machine: Machine{ID: "this-mac", CPUCores: 8, MemoryBytes: 16 * gib, ChipEstates: []string{"cpu"}},
		Machines: []Machine{
			{ID: "this-mac", Name: "This Mac", CPUCores: 8, MemoryBytes: 16 * gib, ChipEstates: []string{"cpu"}},
			{ID: "measured-peer", Name: "Measured peer", CPUCores: 12, MemoryBytes: 32 * gib, ChipEstates: []string{"gpu"}},
		},
		Engines: []Engine{{ID: "local", MachineID: "this-mac", State: "configured"}, {ID: "peer", MachineID: "measured-peer", State: "configured"}},
		Estates: []ChipEstate{{ID: "cpu", Available: true}, {ID: "gpu", Available: true}},
	}
	if _, err := BuildPlanForMachine(catalog, "measured-peer", "local", 4, 8*gib, 0, []string{"gpu"}); err == nil {
		t.Fatal("BuildPlanForMachine() accepted a route bound to another machine")
	}
	if _, err := BuildPlanForMachine(catalog, "measured-peer", "peer", 4, 8*gib, 0, []string{"cpu"}); err == nil {
		t.Fatal("BuildPlanForMachine() accepted an estate outside the selected receipt")
	}
	if _, err := BuildPlanForMachine(catalog, "measured-peer", "peer", 4, 8*gib, 0, []string{"gpu"}); err != nil {
		t.Fatalf("BuildPlanForMachine() rejected the selected receipt: %v", err)
	}
}
