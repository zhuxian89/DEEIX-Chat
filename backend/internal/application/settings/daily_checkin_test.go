package settings

import "testing"

func TestValidatePatchItemDailyCheckinConfig(t *testing.T) {
	valid := PatchItem{
		Namespace: "daily_checkin",
		Key:       "config",
		Value:     `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Asia/Shanghai","prizes":[{"calls":10,"weightBps":10000}]}`,
	}
	if err := validatePatchItem(valid); err != nil {
		t.Fatalf("validatePatchItem(valid) error = %v", err)
	}
	invalid := valid
	invalid.Value = `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Asia/Shanghai","prizes":[{"calls":10,"weightBps":9000}]}`
	if err := validatePatchItem(invalid); err == nil {
		t.Fatal("validatePatchItem(invalid) error = nil")
	}
}
