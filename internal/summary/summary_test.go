package summary_test

import (
	"strings"
	"testing"

	"github.com/issam/ctxd/internal/summary"
)

const phpService = `<?php

namespace App\Services;

/**
 * Build stay-based KPIs and morning report data.
 */
class ReservationOverviewService
{
    public function overview($start, $end, array $filters = []): array
    {
        return [];
    }

    public function morningReport($date, array $filters = [], array $options = []): array
    {
        return [];
    }

    protected function generateMorningReportPdf($date): string
    {
        return '';
    }

    private function helper(): void {}
}
`

func TestPHPClassExtraction(t *testing.T) {
	s := summary.Summarize("app/Services/ReservationOverviewService.php", phpService, "php", nil)
	if s.ClassName != "ReservationOverviewService" {
		t.Errorf("expected class name ReservationOverviewService, got %q", s.ClassName)
	}
	if s.Namespace != `App\Services` {
		t.Errorf("expected namespace App\\Services, got %q", s.Namespace)
	}
	if s.Type != "Service" {
		t.Errorf("expected type Service, got %q", s.Type)
	}
}

func TestPHPMethodExtraction(t *testing.T) {
	s := summary.Summarize("app/Services/ReservationOverviewService.php", phpService, "php", nil)
	names := map[string]bool{}
	for _, m := range s.Methods {
		names[m.Name] = true
	}
	for _, want := range []string{"overview", "morningReport", "generateMorningReportPdf", "helper"} {
		if !names[want] {
			t.Errorf("expected method %q not found in %v", want, s.Methods)
		}
	}
}

func TestPHPDocblockPurpose(t *testing.T) {
	s := summary.Summarize("app/Services/ReservationOverviewService.php", phpService, "php", nil)
	if !strings.Contains(s.Purpose, "KPIs") {
		t.Errorf("expected docblock purpose, got %q", s.Purpose)
	}
}

func TestPHPMatchedFor(t *testing.T) {
	s := summary.Summarize("app/Services/ReservationOverviewService.php", phpService, "php", []string{"morningReport", "notPresent"})
	found := false
	for _, r := range s.MatchedFor {
		if strings.Contains(r, "morningReport") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected morningReport in MatchedFor, got %v", s.MatchedFor)
	}
	for _, r := range s.MatchedFor {
		if strings.Contains(r, "notPresent") {
			t.Error("notPresent should not appear in MatchedFor")
		}
	}
}

func TestLaravelTypeDetection(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"app/Http/Controllers/FooController.php", "Controller"},
		{"app/Services/BarService.php", "Service"},
		{"app/Models/User.php", "Model"},
		{"database/migrations/2024_01_create_users.php", "Migration"},
		{"routes/web.php", "Route file"},
		{"resources/views/dashboard.blade.php", "Blade View"},
		{"tests/Feature/FooTest.php", "Test"},
	}
	for _, tc := range cases {
		s := summary.Summarize(tc.path, "<?php\nclass Foo {}", "php", nil)
		if s.Type != tc.want {
			t.Errorf("path %q: expected type %q, got %q", tc.path, tc.want, s.Type)
		}
	}
}

func TestFormatOutput(t *testing.T) {
	s := summary.Summarize("app/Services/ReservationOverviewService.php", phpService, "php", []string{"morningReport"})
	text := summary.Format(s)
	if !strings.Contains(text, "File:") {
		t.Error("missing File: line")
	}
	if !strings.Contains(text, "Service") {
		t.Error("missing type in output")
	}
	if !strings.Contains(text, "morningReport") {
		t.Error("missing method in output")
	}
}
