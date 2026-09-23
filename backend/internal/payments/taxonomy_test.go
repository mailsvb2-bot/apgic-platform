package payments

import (
	"reflect"
	"testing"
)

func TestSelectionKeepsProviderMethodAndRailSeparate(t *testing.T) {
	typ := reflect.TypeOf(Selection{})
	for _, field := range []string{"ProviderID", "MethodCode", "RailCode"} {
		if _, ok := typ.FieldByName(field); !ok {
			t.Fatalf("missing typed field %s", field)
		}
	}
	if _, mixed := typ.FieldByName("PaymentType"); mixed {
		t.Fatal("mixed payment_type field is forbidden")
	}
}
