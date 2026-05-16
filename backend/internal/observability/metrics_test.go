package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusBucket(t *testing.T) {
	cases := map[string]string{
		"200":   "2xx",
		"201":   "2xx",
		"301":   "3xx",
		"404":   "4xx",
		"500":   "5xx",
		"503":   "5xx",
		"":      "0",
		"abc":   "other",
		"99":    "other",
	}
	for in, want := range cases {
		if got := statusBucket(in); got != want {
			t.Errorf("statusBucket(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMetricsHandler_PublicByDefault(t *testing.T) {
	Register()
	srv := httptest.NewServer(MetricsHandler(""))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// Standard go_* collectors are auto-registered; any one of them
	// confirms promhttp emitted a real payload (and we didn't get a
	// blank 200).
	if !strings.Contains(string(body), "go_goroutines") {
		t.Errorf("body missing go_goroutines metric — promhttp not wired correctly")
	}
}

func TestMetricsHandler_BearerGated(t *testing.T) {
	Register()
	const token = "secret-bearer-token-for-scrape"
	srv := httptest.NewServer(MetricsHandler(token))
	defer srv.Close()

	t.Run("no header → 401", func(t *testing.T) {
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("wrong token → 401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("correct token → 200", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
	})
}
