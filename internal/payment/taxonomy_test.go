package payment

import "testing"

func TestProviderMethodAndRailAreIndependentTypedFields(t *testing.T) {
	route := Route{
		Provider: Provider("provider-x"),
		Method:   Method("CARD"),
		Rail:     Rail("DOMESTIC_CARD_RAIL"),
	}
	if err := route.Validate(); err != nil {
		t.Fatal(err)
	}
	if route.Provider == Provider(route.Method) || route.Method == Method(route.Rail) {
		t.Fatal("provider, method and rail must not collapse into one semantic value")
	}
}
