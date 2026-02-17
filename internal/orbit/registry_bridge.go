package orbit

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/arjenschwarz/orbit/internal/registry"
	"github.com/arjenschwarz/orbit/internal/variants"
	"github.com/google/uuid"
)

// registerRun creates a new registry entry for this orchestration run.
// Returns the run ID and any error. Errors are logged but not fatal (req 3.7).
func (o *Orbit) registerRun() (string, error) {
	if o.registry == nil {
		return "", nil
	}

	entry := registry.NewRunEntry()
	entry.Name = o.config.BranchName
	entry.Repository = registry.GetRepository(o.config.WorkingDir)
	entry.Branch = o.config.BranchName
	entry.Status = registry.StatusRunning
	entry.StartedAt = time.Now()

	// Set log directory (convert to absolute path for web interface access)
	var logDir string
	if o.logManager != nil {
		logDir = o.logManager.SessionDir()
	} else {
		logDir = o.config.LogDir
	}
	absLogDir, err := filepath.Abs(logDir)
	if err != nil {
		log.Printf("Warning: failed to get absolute path for log dir: %v", err)
		absLogDir = logDir // Fall back to original
	}
	entry.LogDir = absLogDir

	// Set run number for file naming (defaults to 1 if no log manager)
	if o.logManager != nil {
		entry.RunNumber = o.logManager.RunNumber()
	} else {
		entry.RunNumber = 1
	}

	// Set PID for auto-registered runs
	pid := os.Getpid()
	entry.PID = &pid

	if err := o.registry.Register(entry); err != nil {
		log.Printf("Warning: failed to register run: %v", err)
		return "", nil
	}

	return entry.ID, nil
}

// registerVariantRun creates registry entries for each variant.
// Each variant gets its own entry with variant-specific metadata.
// Returns the shared variant run ID. Errors are logged but not fatal.
func (o *Orbit) registerVariantRun(variantList []*variants.Variant) string {
	if o.registry == nil {
		return ""
	}

	// Generate a shared ID to group all variants from this run
	variantRunID := uuid.NewString()
	o.variantRunID = variantRunID
	o.variantRegistryIDs = make(map[int]string)

	pid := os.Getpid()
	now := time.Now()
	variantTotal := len(variantList)

	for _, v := range variantList {
		entry := registry.NewRunEntry()
		entry.Name = fmt.Sprintf("%s [variant %d/%d]", o.config.BranchName, v.ID, variantTotal)
		entry.Repository = registry.GetRepository(o.config.WorkingDir)
		entry.Branch = o.config.BranchName
		entry.Status = registry.StatusRunning
		entry.StartedAt = now
		entry.PID = &pid
		entry.RunNumber = 1

		// Set variant-specific fields
		entry.IsVariant = true
		entry.VariantID = v.ID
		entry.VariantRunID = variantRunID
		entry.VariantTotal = variantTotal
		entry.VariantAgent = v.Agent
		entry.VariantBranch = v.Branch

		// Set log directory to this variant's log directory
		variantLogDir := filepath.Join(o.config.SpecDir, ".orbit", "logs", fmt.Sprintf("variant-%d", v.ID))
		absLogDir, err := filepath.Abs(variantLogDir)
		if err != nil {
			log.Printf("Warning: failed to get absolute path for variant %d log dir: %v", v.ID, err)
			absLogDir = variantLogDir
		}
		entry.LogDir = absLogDir

		if err := o.registry.Register(entry); err != nil {
			log.Printf("Warning: failed to register variant %d: %v", v.ID, err)
			continue
		}

		o.variantRegistryIDs[v.ID] = entry.ID
	}

	return variantRunID
}

// updateVariantRegistryStatus updates a variant's status in the registry.
// Failures are logged but not fatal.
func (o *Orbit) updateVariantRegistryStatus(variantID int, status registry.RunStatus) {
	if o.registry == nil || o.variantRegistryIDs == nil {
		return
	}

	registryID, ok := o.variantRegistryIDs[variantID]
	if !ok {
		return
	}

	entry, err := o.registry.Get(registryID)
	if err != nil {
		log.Printf("Warning: failed to get variant %d registry entry: %v", variantID, err)
		return
	}
	if entry == nil {
		return
	}

	entry.Status = status
	if status == registry.StatusCompleted || status == registry.StatusFailed {
		now := time.Now()
		entry.FinishedAt = &now
	}

	if err := o.registry.Register(entry); err != nil {
		log.Printf("Warning: failed to update variant %d status: %v", variantID, err)
	}
}

// updatePhaseStatus updates the phase status in the registry.
// Failures are logged but not fatal (req 3.7).
func (o *Orbit) updatePhaseStatus(phaseNum int, status registry.PhaseStatus, runCount int) {
	if o.registry == nil || o.runID == "" {
		return
	}

	phase := registry.Phase{
		Number:   phaseNum,
		Status:   status,
		RunCount: runCount,
	}

	if err := o.registry.UpdatePhase(o.runID, phase); err != nil {
		log.Printf("Warning: failed to update phase status: %v", err)
	}
}

// updateRunStatus updates the run status in the registry.
// Failures are logged but not fatal (req 3.7).
func (o *Orbit) updateRunStatus(status registry.RunStatus) {
	if o.registry == nil || o.runID == "" {
		return
	}

	entry, err := o.registry.Get(o.runID)
	if err != nil {
		log.Printf("Warning: failed to get run for status update: %v", err)
		return
	}
	if entry == nil {
		log.Printf("Warning: run entry not found for status update: %s", o.runID)
		return
	}

	entry.Status = status
	now := time.Now()
	entry.FinishedAt = &now

	if err := o.registry.Register(entry); err != nil {
		log.Printf("Warning: failed to update run status: %v", err)
	}
}

