package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sub2api-desktop/core/internal/gateway"
	"sub2api-desktop/core/internal/store"
)

type settingsPatchAccess struct {
	value store.Settings
}

func (s *settingsPatchAccess) Get() store.Settings { return s.value }
func (s *settingsPatchAccess) Save(value store.Settings) error {
	s.value = value
	return nil
}

func TestPutSettingsPreservesOmittedBooleans(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantAllowLAN bool
	}{
		{name: "omitted booleans", body: `{"language":"en"}`, wantAllowLAN: true},
		{name: "explicit false", body: `{"allow_lan":false}`, wantAllowLAN: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &settingsPatchAccess{value: store.Settings{
				ListenPort: 8080, LocalAPIKey: "local-key", DefaultModel: "gpt-5.4", UserAgent: "test",
				Originator: "codex_cli_rs", Language: "zh-CN", AccountStrategy: gateway.StrategyQuotaAware,
				MaxLogRows: 100000, CompatProfile: "codex", AllowLAN: true, InjectInstr: true,
				AutoStartServer: true, TLSFingerprint: true, AutoRecovery: true,
			}}
			control := &Control{settings: settings}
			request := httptest.NewRequest(http.MethodPut, "/control/settings", strings.NewReader(tt.body))
			response := httptest.NewRecorder()

			control.putSettings(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			got := settings.value
			if got.AllowLAN != tt.wantAllowLAN {
				t.Fatalf("allow_lan = %t, want %t", got.AllowLAN, tt.wantAllowLAN)
			}
			if !got.InjectInstr || !got.AutoStartServer || !got.TLSFingerprint || !got.AutoRecovery {
				t.Fatal("one or more omitted booleans were reset")
			}
		})
	}
}
