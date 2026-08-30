package config

import "testing"

func hasAdvisory(advisories []Advisory, code string) bool {
	for _, a := range advisories {
		if a.Code == code {
			return true
		}
	}
	return false
}

func TestSecurityAdvisoriesFlagsInsecureBaseURL(t *testing.T) {
	c := Default()
	c.Public.BaseURL = "http://example.com"
	c.Mode = "production"

	advisories := c.SecurityAdvisories()
	if !hasAdvisory(advisories, "insecure_base_url") {
		t.Fatal("expected insecure_base_url advisory for a non-https base_url")
	}
	for _, a := range advisories {
		if a.Code == "insecure_base_url" && a.Severity != "critical" {
			t.Fatalf("expected critical severity in production mode, got %q", a.Severity)
		}
	}
}

func TestSecurityAdvisoriesInsecureBaseURLIsOnlyWarningOutsideProduction(t *testing.T) {
	c := Default()
	c.Public.BaseURL = "http://example.com"
	c.Mode = "development"

	advisories := c.SecurityAdvisories()
	found := false
	for _, a := range advisories {
		if a.Code == "insecure_base_url" {
			found = true
			if a.Severity != "warning" {
				t.Fatalf("expected warning severity outside production, got %q", a.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected insecure_base_url advisory even in development mode")
	}
}

func TestSecurityAdvisoriesNoInsecureBaseURLAdvisoryWhenHTTPS(t *testing.T) {
	c := Default()
	c.Public.BaseURL = "https://example.com"
	c.Mode = "production"

	if hasAdvisory(c.SecurityAdvisories(), "insecure_base_url") {
		t.Fatal("did not expect insecure_base_url advisory when base_url is https")
	}
}

func TestSecurityAdvisoriesFlagsDevelopmentMode(t *testing.T) {
	c := Default()
	c.Mode = "development"
	if !hasAdvisory(c.SecurityAdvisories(), "development_mode") {
		t.Fatal("expected development_mode advisory when mode is development")
	}

	c.Mode = "production"
	c.Public.BaseURL = "https://example.com"
	if hasAdvisory(c.SecurityAdvisories(), "development_mode") {
		t.Fatal("did not expect development_mode advisory in production mode")
	}
}
