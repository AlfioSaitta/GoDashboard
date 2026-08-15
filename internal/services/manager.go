package services

import (
	"context"
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
	
	if svc, ok := cfg.GetService("neuronet"); ok {
		sm.neuronet = NewNeuroNetClient(svc)
	}
	if svc, ok := cfg.GetService("minecraft"); ok {
		sm.minecraft = NewMinecraftClient(svc)
	}
	if svc, ok := cfg.GetService("slotbuilder"); ok {
		sm.slotbuilder = NewSlotBuilderClient(svc)
	}
	return sm
}

func (sm *ServiceManager) CheckAllHealth(ctx context.Context) []models.ServiceStatus {
	var statuses []models.ServiceStatus
	
	if sm.neuronet != nil {
		statuses = append(statuses, sm.checkNeuroNet(ctx))
	}
	if sm.minecraft != nil {
		statuses = append(statuses, sm.checkMinecraft(ctx))
	}
	if sm.slotbuilder != nil {
		statuses = append(statuses, sm.checkSlotBuilder(ctx))
	}
	
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

func (sm *ServiceManager) GetNeuroNetDashboard(ctx context.Context) (*models.NeuroNetDashboard, error) {
	if sm.neuronet == nil {
		return nil, nil
	}
	
	nnModels, _ := sm.neuronet.ListModels(ctx)
	training, _ := sm.neuronet.ListTrainingJobs(ctx)
	health, _ := sm.neuronet.Health(ctx)
	
	return &models.NeuroNetDashboard{
		Models:   nnModels,
		Training: training,
		Health:   health,
	}, nil
}

func (sm *ServiceManager) GetMinecraftDashboard(ctx context.Context) (*models.MinecraftDashboard, error) {
	if sm.minecraft == nil {
		return nil, nil
	}
	
	servers, _ := sm.minecraft.ListServers(ctx)
	players, _ := sm.minecraft.ListPlayers(ctx)
	status, _ := sm.minecraft.Status(ctx)
	
	return &models.MinecraftDashboard{
		Servers: servers,
		Players: players,
		Status:  status,
	}, nil
}

func (sm *ServiceManager) GetSlotBuilderDashboard(ctx context.Context) (*models.SlotBuilderDashboard, error) {
	if sm.slotbuilder == nil {
		return nil, nil
	}
	
	games, _ := sm.slotbuilder.ListGames(ctx)
	deployments, _ := sm.slotbuilder.ListDeployments(ctx)
	
	return &models.SlotBuilderDashboard{
		Games:       games,
		Deployments: deployments,
	}, nil
}

func (sm *ServiceManager) GetFullDashboard(ctx context.Context) (*models.DashboardData, error) {
	services := sm.CheckAllHealth(ctx)
	
	neuronet, _ := sm.GetNeuroNetDashboard(ctx)
	minecraft, _ := sm.GetMinecraftDashboard(ctx)
	slotbuilder, _ := sm.GetSlotBuilderDashboard(ctx)
	
	return &models.DashboardData{
		Services:     services,
		NeuroNet:     neuronet,
		Minecraft:    minecraft,
		SlotBuilder:  slotbuilder,
	}, nil
}