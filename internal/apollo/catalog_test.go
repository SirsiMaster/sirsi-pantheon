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

func TestBuildPlanRejectsUnavailableOrDuplicateEstate(t *testing.T) {
	catalog := Catalog{
		Machine: Machine{ID: "this-mac", CPUCores: 8, MemoryBytes: 16 * gib},
		Engines: []Engine{{ID: "apollo-local-sne", State: "configured"}},
		Estates: []ChipEstate{{ID: "cpu", Available: true}, {ID: "gpu", Available: false}},
	}
	if _, err := BuildPlan(catalog, "apollo-local-sne", 4, 8*gib, 0, []string{"gpu"}); err == nil {
		t.Fatal("BuildPlan() accepted an unavailable estate")
	}
	if _, err := BuildPlan(catalog, "apollo-local-sne", 4, 8*gib, 0, []string{"cpu", "cpu"}); err == nil {
		t.Fatal("BuildPlan() accepted a duplicate estate")
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
