package dailycheckin

import "testing"

func TestParseConfigAcceptsDefaultPrizePool(t *testing.T) {
	config, err := ParseConfig(DefaultConfigJSON())
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if !config.Enabled || config.UnitPriceNanousd != 1_670_000 || config.Timezone != "Asia/Shanghai" {
		t.Fatalf("ParseConfig() = %#v", config)
	}
	if len(config.Prizes) != 6 || config.Prizes[5].Calls != 200 {
		t.Fatalf("ParseConfig() prizes = %#v", config.Prizes)
	}
}

func TestParseConfigRejectsInvalidPrizePools(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "duplicate calls", value: `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Asia/Shanghai","prizes":[{"calls":10,"weightBps":5000},{"calls":10,"weightBps":5000}]}`},
		{name: "weight total", value: `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Asia/Shanghai","prizes":[{"calls":10,"weightBps":9999}]}`},
		{name: "invalid timezone", value: `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Mars/Base","prizes":[{"calls":10,"weightBps":10000}]}`},
		{name: "fractional calls", value: `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Asia/Shanghai","prizes":[{"calls":1.5,"weightBps":10000}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseConfig(test.value); err == nil {
				t.Fatal("ParseConfig() error = nil, want validation error")
			}
		})
	}
}
