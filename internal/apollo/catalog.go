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
	// ChipEstates binds detected, selectable estates to this capacity receipt. An
	// estate can be selectable before it is currently SNE-qualified; Plan makes
	// that qualification gap explicit rather than silently hiding the estate.
	// Empty is
	// accepted only for the legacy single-machine projection, where Catalog's
	// top-level estate list remains authoritative.
	ChipEstates []string `json:"chip_estates,omitempty"`
	// Estates is the machine-specific typed estate receipt. It lets a selected
	// Horus instance describe its own CPU, GPU, memory and Neural Engine state
	// instead of inheriting details observed on this device. ChipEstates remains
	// for v2-compatible consumers and must agree when both are present.
	Estates []ChipEstate `json:"estates,omitempty"`
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
	// UnavailableEstates is a subset of ChipEstates whose selection is retained
	// for operator intent, but which SNE must qualify before it can admit work.
	UnavailableEstates []string `json:"unavailable_chip_estates,omitempty"`
	Execution          string   `json:"execution"`
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
	machine := Machine{ID: "this-mac", Name: machineName, CPUCores: hw.CPUCores, MemoryBytes: hw.TotalRAM, ChipEstates: estateIDs(estates), Estates: estates}
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
	enumerated, err := estatesForMachine(machine, c.Estates)
	if err != nil {
		return Plan{}, err
	}
	seen := map[string]bool{}
	unavailable := make([]string, 0)
	for _, estate := range estates {
		observed, ok := enumerated[estate]
		if !ok {
			return Plan{}, fmt.Errorf("%q is not a detected chip estate on %s", estate, machine.Name)
		}
		if seen[estate] {
			return Plan{}, fmt.Errorf("chip estate %q was selected more than once", estate)
		}
		seen[estate] = true
		if !observed.Available {
			unavailable = append(unavailable, estate)
		}
	}
	if len(estates) == 0 {
		return Plan{}, fmt.Errorf("choose at least one detected chip estate")
	}
	execution := "planned; SNE admission is required before inference starts"
	if len(unavailable) > 0 {
		execution = "planned with requested estates awaiting SNE qualification; inference cannot start until SNE admits them"
	}
	return Plan{SchemaVersion: "apollo-plan/v1", MachineID: machine.ID, EngineID: engine.ID, ResidentModel: engine.ResidentModel, CPUCores: cores, MemoryBytes: memoryBytes, SwapBytes: swapBytes, ChipEstates: estates, UnavailableEstates: unavailable, Execution: execution}, nil
}

// estatesForMachine returns the exact chip-estate receipt for one machine.
// New multi-Horus catalogs carry an owned estate record on Machine; the
// top-level list remains a legacy local compatibility projection. When both
// encodings occur they must describe the same set, otherwise planning fails
// rather than borrowing an estate from another device.
func estatesForMachine(machine Machine, fallback []ChipEstate) (map[string]ChipEstate, error) {
	fromMachine := machine.Estates
	if len(fromMachine) == 0 {
		allowed := map[string]bool{}
		for _, id := range machine.ChipEstates {
			if strings.TrimSpace(id) == "" {
				return nil, fmt.Errorf("the selected machine has an invalid chip estate identity")
			}
			if allowed[id] {
				return nil, fmt.Errorf("the selected machine repeats chip estate %q", id)
			}
			allowed[id] = true
		}
		fromMachine = make([]ChipEstate, 0, len(fallback))
		for _, estate := range fallback {
			if len(allowed) == 0 || allowed[estate.ID] {
				fromMachine = append(fromMachine, estate)
			}
		}
	}

	enumerated := make(map[string]ChipEstate, len(fromMachine))
	for _, estate := range fromMachine {
		if strings.TrimSpace(estate.ID) == "" || strings.TrimSpace(estate.Name) == "" {
			return nil, fmt.Errorf("the selected machine has an incomplete chip estate receipt")
		}
		if _, duplicate := enumerated[estate.ID]; duplicate {
			return nil, fmt.Errorf("the selected machine repeats chip estate %q", estate.ID)
		}
		enumerated[estate.ID] = estate
	}
	if len(enumerated) == 0 {
		return nil, fmt.Errorf("the selected machine did not enumerate chip estates")
	}
	if len(machine.Estates) > 0 && len(machine.ChipEstates) > 0 {
		if len(machine.ChipEstates) != len(enumerated) {
			return nil, fmt.Errorf("the selected machine has conflicting chip estate receipts")
		}
		for _, id := range machine.ChipEstates {
			if _, ok := enumerated[id]; !ok {
				return nil, fmt.Errorf("the selected machine has conflicting chip estate receipts")
			}
		}
	}
	return enumerated, nil
}

func estateIDs(estates []ChipEstate) []string {
	ids := make([]string, 0, len(estates))
	for _, estate := range estates {
		if strings.TrimSpace(estate.ID) != "" {
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
