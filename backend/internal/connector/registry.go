package connector

import (
	"errors"
	"strings"
)

type CapabilityClass string

const (
	CapabilityGrowthCRM              CapabilityClass = "GROWTH_CRM_PROVIDER"
	CapabilityCommunication          CapabilityClass = "COMMUNICATION_PROVIDER"
	CapabilityPersona                CapabilityClass = "PERSONA_PROVIDER"
	CapabilityManagementIntelligence CapabilityClass = "MANAGEMENT_INTELLIGENCE_PROVIDER"
	CapabilityNotification           CapabilityClass = "NOTIFICATION_PROVIDER"
	CapabilityCalendar               CapabilityClass = "CALENDAR_PROVIDER"
	CapabilityPayment                CapabilityClass = "PAYMENT_PROVIDER"
	CapabilityStorage                CapabilityClass = "STORAGE_PROVIDER"
	CapabilityCDN                    CapabilityClass = "CDN_PROVIDER"
	CapabilityAnalytics              CapabilityClass = "ANALYTICS_PROVIDER"
	CapabilityLMS                    CapabilityClass = "LMS_PROVIDER"
	CapabilityERP                    CapabilityClass = "ERP_PROVIDER"
	CapabilityIdentity               CapabilityClass = "IDENTITY_PROVIDER"
	CapabilityAIModel                CapabilityClass = "AI_MODEL_PROVIDER"
)

var validCapabilityClasses = map[CapabilityClass]struct{}{
	CapabilityGrowthCRM:              {},
	CapabilityCommunication:          {},
	CapabilityPersona:                {},
	CapabilityManagementIntelligence: {},
	CapabilityNotification:           {},
	CapabilityCalendar:               {},
	CapabilityPayment:                {},
	CapabilityStorage:                {},
	CapabilityCDN:                    {},
	CapabilityAnalytics:              {},
	CapabilityLMS:                    {},
	CapabilityERP:                    {},
	CapabilityIdentity:               {},
	CapabilityAIModel:                {},
}

func (c CapabilityClass) Valid() bool {
	_, ok := validCapabilityClasses[c]
	return ok
}

func ExecuteScopeFor(capability CapabilityClass) string {
	if !capability.Valid() {
		return ""
	}
	return "connector:execute:" + string(capability)
}

type Status string

const (
	StatusConfiguring Status = "CONFIGURING"
	StatusActive      Status = "ACTIVE"
	StatusDegraded    Status = "DEGRADED"
	StatusDisabled    Status = "DISABLED"
)

func (s Status) Valid() bool {
	switch s {
	case StatusConfiguring, StatusActive, StatusDegraded, StatusDisabled:
		return true
	default:
		return false
	}
}

var ErrInvalidConnector = errors.New("connector id, registered capability, provider kind and config ref are required")

type Instance struct {
	ID           string
	Capability   CapabilityClass
	ProviderKind string
	Status       Status
	ConfigRef    string
	ExecuteScope string
}

func NewInstance(id string, capability CapabilityClass, providerKind, configRef string) (Instance, error) {
	id = strings.TrimSpace(id)
	providerKind = strings.TrimSpace(providerKind)
	configRef = strings.TrimSpace(configRef)
	executeScope := ExecuteScopeFor(capability)
	if id == "" || executeScope == "" || providerKind == "" || configRef == "" {
		return Instance{}, ErrInvalidConnector
	}
	return Instance{
		ID:           id,
		Capability:   capability,
		ProviderKind: providerKind,
		Status:       StatusConfiguring,
		ConfigRef:    configRef,
		ExecuteScope: executeScope,
	}, nil
}
