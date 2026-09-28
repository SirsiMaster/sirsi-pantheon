// Package apollo describes the local inference capacity Pantheon can plan
// against. It deliberately observes and validates; model execution remains the
// SNE runtime's responsibility.
package apollo

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/provider"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

const gib = int64(1024 * 1024 * 1024)

// Catalog is the typed source used by Stack Lab. It has no candidate model or
// machine guessing: configured routes are explicit, unavailable routes remain
// visibly unconfigured, and every estate is detected by Seba.
type Catalog struct {
	SchemaVersion string `json:"schema_version"`
	// Machine remains the current local capacity projection for older consumers.
	// Machines is the selectable list consumed by Stack Lab. It contains only
	// typed capacity receipts; Pantheon never turns a discovered peer name into
	// a usable compute target.
	Machine  Machine      `json:"machine"`
	Machines []Machine    `json:"machines"`
	Engines  []Engine     `json:"engines"`
	Estates  []ChipEstate `json:"chip_estates"`
}

type Machine struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CPUCores    int    `json:"cpu_cores"`
	MemoryBytes int64  `json:"memory_bytes"`
	// ChipEstates binds selectable estates to this capacity receipt. Empty is
	// accepted only for the legacy single-machine projection, where Catalog's
	// top-level estate list remains authoritative.
	ChipEstates []string `json:"chip_estates,omitempty"`
}

type Engine struct {
	ID            string `json:"id"`
	MachineID     string `json:"machine_id,omitempty"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	ResidentModel string `json:"resident_model,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	State         string `json:"state"` // configured|unconfigured
}

type ChipEstate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	Description string `json:"description"`
}

// Plan is a non-mutating, validated declaration of a requested Apollo run.
// It is intentionally not an execution receipt: SNE must separately accept a
// plan before a model can reserve local compute.
type Plan struct {
	SchemaVersion string   `json:"schema_version"`
	MachineID     string   `json:"machine_id"`
	EngineID      string   `json:"engine_id"`
	ResidentModel string   `json:"resident_model,omitempty"`
	CPUCores      int      `json:"cpu_cores"`
	MemoryBytes   int64    `json:"memory_bytes"`
	SwapBytes     int64    `json:"swap_bytes"`
	ChipEstates   []string `json:"chip_estates"`
	Execution     string   `json:"execution"`
}

// Collect returns the current machine and the one configured local SNE route.
// A missing route remains visible as unconfigured; Stack Lab never displays a
// plausible model that is not configured on the local device.
func Collect(home string) (Catalog, error) {
	hw, err := seba.DetectHardware()
	if err != nil {
		return Catalog{}, fmt.Errorf("detect local hardware: %w", err)
	}
	machineName := strings.TrimSpace(hw.CPUModel)
	if machineName == "" {
		machineName = "This Mac"
	}
	conf := provider.LoadConf(home)
	local := provider.Local(home, conf)
	engine := Engine{ID: "apollo-local-sne", MachineID: "this-mac", Name: "Apollo local inference", Provider: "SNE", State: "unconfigured"}
	if local != nil {
		engine.State = "configured"
		engine.ResidentModel = strings.TrimSpace(local.Model)
		engine.Endpoint = strings.TrimSpace(local.Endpoint)
	}
	estates := []ChipEstate{
		{ID: "cpu", Name: "CPU", Available: hw.CPUCores > 0, Description: fmt.Sprintf("%d logical cores", hw.CPUCores)},
		{ID: "unified-memory", Name: "Unified memory", Available: hw.TotalRAM > 0, Description: seba.FormatBytes(hw.TotalRAM)},
		{ID: "gpu", Name: "GPU", Available: hw.GPU.Type != seba.GPUNone && hw.GPU.Name != "", Description: nonEmpty(hw.GPU.Name, "No detected GPU")},
		{ID: "neural-engine", Name: "Neural Engine", Available: hw.NeuralEngine, Description: ternary(hw.NeuralEngine, "Detected on this Mac", "Not detected")},
	}
	machine := Machine{ID: "this-mac", Name: machineName, CPUCores: hw.CPUCores, MemoryBytes: hw.TotalRAM, ChipEstates: estateIDs(estates)}
	return Catalog{SchemaVersion: "apollo-catalog/v2", Machine: machine, Machines: []Machine{machine}, Engines: []Engine{engine}, Estates: estates}, nil
}

// BuildPlan validates a user-selected resource envelope without reserving
// memory, starting a process, or changing the active inference service.
func BuildPlan(c Catalog, engineID string, cores int, memoryBytes, swapBytes int64, estates []string) (Plan, error) {
	return BuildPlanForMachine(c, c.Machine.ID, engineID, cores, memoryBytes, swapBytes, estates)
}

// BuildPlanForMachine validates the exact resource envelope against a selected
// capacity receipt. A peer becomes selectable only when it is present in
// Catalog.Machines; an ambient Ra/Hermes name cannot borrow this Mac's limits.
func BuildPlanForMachine(c Catalog, machineID, engineID string, cores int, memoryBytes, swapBytes int64, estates []string) (Plan, error) {
	machine, ok := c.machineByID(machineID)
	if !ok {
		return Plan{}, fmt.Errorf("the selected machine has no typed Apollo capacity receipt")
	}
	if machine.CPUCores < 1 || machine.MemoryBytes < gib {
		return Plan{}, fmt.Errorf("the selected machine did not report a usable CPU and memory capacity")
	}
	if engineID == "" {
		return Plan{}, fmt.Errorf("choose a configured Apollo inference engine")
	}
	var engine *Engine
	for i := range c.Engines {
		if c.Engines[i].ID == engineID {
			engine = &c.Engines[i]
			break
		}
	}
	if engine == nil || engine.State != "configured" {
		return Plan{}, fmt.Errorf("the selected Apollo engine is not configured on this Mac")
	}
	if engine.MachineID != "" && engine.MachineID != machine.ID {
		return Plan{}, fmt.Errorf("the selected Apollo engine has no configured route on %s", machine.Name)
	}
	if cores < 1 || cores > machine.CPUCores {
		return Plan{}, fmt.Errorf("CPU cores must be between 1 and %d", machine.CPUCores)
	}
	if memoryBytes < gib || memoryBytes > machine.MemoryBytes {
		return Plan{}, fmt.Errorf("memory must be between 1 GiB and %s", seba.FormatBytes(machine.MemoryBytes))
	}
	if swapBytes < 0 || swapBytes > machine.MemoryBytes {
		return Plan{}, fmt.Errorf("swap target must be between 0 and %s", seba.FormatBytes(machine.MemoryBytes))
	}
	allowedEstates := map[string]bool{}
	for _, id := range machine.ChipEstates {
		allowedEstates[id] = true
	}
	available := map[string]bool{}
	for _, estate := range c.Estates {
		if estate.Available && (len(allowedEstates) == 0 || allowedEstates[estate.ID]) {
			available[estate.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, estate := range estates {
		if !available[estate] {
			return Plan{}, fmt.Errorf("%q is not an available chip estate on this Mac", estate)
		}
		if seen[estate] {
			return Plan{}, fmt.Errorf("chip estate %q was selected more than once", estate)
		}
		seen[estate] = true
	}
	if len(estates) == 0 {
		return Plan{}, fmt.Errorf("choose at least one available chip estate")
	}
	return Plan{SchemaVersion: "apollo-plan/v1", MachineID: machine.ID, EngineID: engine.ID, ResidentModel: engine.ResidentModel, CPUCores: cores, MemoryBytes: memoryBytes, SwapBytes: swapBytes, ChipEstates: estates, Execution: "planned; SNE admission is required before inference starts"}, nil
}

func estateIDs(estates []ChipEstate) []string {
	ids := make([]string, 0, len(estates))
	for _, estate := range estates {
		if estate.Available {
			ids = append(ids, estate.ID)
		}
	}
	return ids
}

func (c Catalog) machineByID(id string) (Machine, bool) {
	if id == "" {
		return Machine{}, false
	}
	machines := c.Machines
	if len(machines) == 0 {
		machines = []Machine{c.Machine}
	}
	for _, machine := range machines {
		if machine.ID == id {
			return machine, true
		}
	}
	return Machine{}, false
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
func ternary(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
