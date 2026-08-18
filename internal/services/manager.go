package services

import (
	"context"
	"sync"
	"time"

	"dashboard/internal/config"
	"dashboard/internal/models"
)

type ServiceManager struct {
	neuronet    *NeuroNetClient
	minecraft   *MinecraftClient
	slotbuilder *SlotBuilderClient
	config      *config.Config
}

func NewServiceManager(cfg *config.Config) *ServiceManager {
	sm := &ServiceManager{config: cfg}
	sm.rebuildClients(cfg)
	return sm
}

// Reconfigure rebuilds the per-service clients from a (possibly updated)
// config. It keeps the same logical instance so callers holding a pointer
// (e.g. DashboardAPI) observe the new endpoints/auth without re-wiring.
func (sm *ServiceManager) Reconfigure(cfg *config.Config) {
	sm.config = cfg
	sm.rebuildClients(cfg)
}

func (sm *ServiceManager) rebuildClients(cfg *config.Config) {
	if svc, ok := cfg.GetService("neuronet"); ok {
		sm.neuronet = NewNeuroNetClient(svc)
	} else {
		sm.neuronet = nil
	}
	if svc, ok := cfg.GetService("minecraft"); ok {
		sm.minecraft = NewMinecraftClient(svc)
	} else {
		sm.minecraft = nil
	}
	if svc, ok := cfg.GetService("slotbuilder"); ok {
		sm.slotbuilder = NewSlotBuilderClient(svc)
	} else {
		sm.slotbuilder = nil
	}
}

func (sm *ServiceManager) CheckAllHealth(ctx context.Context) []models.ServiceStatus {
	var wg sync.WaitGroup
	var mu sync.Mutex
	statuses := make([]models.ServiceStatus, 0, 3)

	check := func(fn func(context.Context) models.ServiceStatus) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := fn(ctx)
			mu.Lock()
			statuses = append(statuses, s)
			mu.Unlock()
		}()
	}

	if sm.neuronet != nil {
		check(sm.checkNeuroNet)
	}
	if sm.minecraft != nil {
		check(sm.checkMinecraft)
	}
	if sm.slotbuilder != nil {
		check(sm.checkSlotBuilder)
	}
	wg.Wait()
	return statuses
}

func (sm *ServiceManager) checkNeuroNet(ctx context.Context) models.ServiceStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	health, err := sm.neuronet.Health(ctx)
	status := models.ServiceStatus{
		ID:        "neuronet",
		Name:      "NeuroNet",
		LastCheck: time.Now(),
		Details:   health,
	}
	if err != nil {
		status.Healthy = false
		status.Error = err.Error()
	} else {
		status.Healthy = true
	}
	return status
}

func (sm *ServiceManager) checkMinecraft(ctx context.Context) models.ServiceStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	statusData, err := sm.minecraft.Status(ctx)
	status := models.ServiceStatus{
		ID:        "minecraft",
		Name:      "Minecraft Network",
		LastCheck: time.Now(),
		Details:   statusData,
	}
	if err != nil {
		status.Healthy = false
		status.Error = err.Error()
	} else {
		status.Healthy = true
	}
	return status
}

func (sm *ServiceManager) checkSlotBuilder(ctx context.Context) models.ServiceStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	games, err := sm.slotbuilder.ListGames(ctx)
	status := models.ServiceStatus{
		ID:        "slotbuilder",
		Name:      "SlotBuilder",
		LastCheck: time.Now(),
		Details:   map[string]interface{}{"games_count": len(games)},
	}
	if err != nil {
		status.Healthy = false
		status.Error = err.Error()
	} else {
		status.Healthy = true
	}
	return status
}
