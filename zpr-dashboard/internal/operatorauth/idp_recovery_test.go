package operatorauth

import (
	"bytes"
	"html/template"
	"os"
	"strings"
	"testing"
)

func TestLocalIDPExpiredPageRecoveryTemplate(t *testing.T) {
	content, err := os.ReadFile("../../scripts/operator-idp/error.html")
	if err != nil {
		t.Fatal(err)
	}
	page, err := template.New("error.html").Parse(`{{define "header.html"}}<!doctype html>{{end}}{{define "footer.html"}}{{end}}` + string(content))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, message string
		expired       bool
	}{
		{"expired-refresh", "Requested resource does not exist.", true},
		{"provider-failure", "Internal server error.", false},
		{"escaped-provider-error", "<script>unsafe</script>", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := page.Execute(&output, struct{ ErrType, ErrMsg string }{"Bad Request", scenario.message}); err != nil {
				t.Fatal(err)
			}
			body := output.String()
			if scenario.expired {
				for _, expected := range []string{"Sign-in timed out", "Timed out. Try again.", `href="https://localhost:8787/"`, `referrerpolicy="no-referrer"`} {
					if !strings.Contains(body, expected) {
						t.Fatalf("missing recovery content: %s", expected)
					}
				}
				if strings.Contains(body, "Bad Request") || strings.Contains(body, "Requested resource does not exist.") {
					t.Fatal("expired login still displays the generic provider error")
				}
			} else if strings.Contains(body, "Timed out.") || !strings.Contains(body, template.HTMLEscapeString(scenario.message)) {
				t.Fatal("other provider error was hidden or not escaped")
			}
		})
	}
}
