package connector

import "errors"

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

type Status string

const (
	StatusConfiguring Status = "CONFIGURING"
	StatusActive      Status = "ACTIVE"
	StatusDegraded    Status = "DEGRADED"
	StatusDisabled    Status = "DISABLED"
)

var ErrInvalidConnector = errors.New("connector id, capability and provider kind are required")

type Instance struct {
	ID           string
	Capability   CapabilityClass
	ProviderKind string
	Status       Status
	ConfigRef    string
}

func NewInstance(id string, capability CapabilityClass, providerKind, configRef string) (Instance, error) {
	if id == "" || capability == "" || providerKind == "" {
		return Instance{}, ErrInvalidConnector
	}
	return Instance{
		ID: id, Capability: capability, ProviderKind: providerKind,
		Status: StatusConfiguring, ConfigRef: configRef,
	}, nil
}
