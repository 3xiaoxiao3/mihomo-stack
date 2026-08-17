package candidate

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"
)

type staticBuilder []byte

func (b staticBuilder) Build(context.Context) ([]byte, error) { return b, nil }

func TestOverlayEnforcesOnlyRuntimeSettings(t *testing.T) {
	overlay := NewOverlay(staticBuilder(`
mixed-port: 1234
allow-lan: false
external-controller: 127.0.0.1:9090
secret: subscription-secret
mode: rule
proxies:
  - name: keep-me
    type: ss
`), RuntimeSettings{
		Enabled:            true,
		MixedPort:          7890,
		AllowLAN:           true,
		ExternalController: "0.0.0.0:9090",
		ExternalUI:         "/app/dashboard",
		Secret:             "operator-secret",
	})

	result, err := overlay.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(result, &document); err != nil {
		t.Fatal(err)
	}
	if document["mixed-port"] != 7890 || document["allow-lan"] != true {
		t.Fatalf("runtime settings = %#v", document)
	}
	if document["external-controller"] != "0.0.0.0:9090" || document["secret"] != "operator-secret" {
		t.Fatalf("controller settings = %#v", document)
	}
	if document["external-ui"] != "/app/dashboard" {
		t.Fatalf("external UI = %#v", document["external-ui"])
	}
	if document["mode"] != "rule" || len(document["proxies"].([]any)) != 1 {
		t.Fatalf("subscription content was not preserved: %#v", document)
	}
}
