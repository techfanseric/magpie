//go:build darwin && cgo

package gui

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// Exported previews come from the same AppKit drawing path as the menu bar,
// with fixture subscriptions only, never a user's accounts or credentials.
func TestTrayRingNativeWidthAppearanceAndClicks(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	reset := now.Add(3 * time.Hour)
	cards := []provider.SubscriptionQuota{}
	for i, name := range []string{"Codex", "Kimi", "MiniMax", "GLM"} {
		cards = append(cards, provider.SubscriptionQuota{Provider: name, Name: name, Icon: []string{"codex-color", "kimi", "minimax-color", "zhipu-color"}[i], Windows: []provider.QuotaWindow{
			{Name: "5-hour", Used: []float64{35, 58, 92, 24}[i], Span: 5 * time.Hour, ResetsAt: &reset},
			{Name: "Weekly", Used: 45, Span: 7 * 24 * time.Hour},
		}})
	}
	cells, _, _ := trayUsageView(cards, now, true, false)
	rings := trayRingCells(cells, cards, now, true, true, nil)
	for _, dark := range []bool{false, true} {
		for _, scale := range []float64{1, 2, 3} {
			b, w, h := trayImagePNG(rings, nil, 22, scale, dark, true)
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil || h != int(22*scale) || w != int(95*scale) {
				t.Fatalf("ring strip at %gx: %dx%d, %v", scale, w, h, err)
			}
			if img.Bounds().Dx() != w {
				t.Fatal("PNG width differs")
			}
			_, textWidth, _ := trayImagePNG(cells, nil, 22, scale, dark, true)
			if w >= textWidth {
				t.Fatalf("four rings use %d px vs %d for numbers", w, textWidth)
			}
			for i := range rings {
				x := trayImageCellStart(rings, 22, i)
				if at := trayImageCellAt(rings, 22, x+11); at != i {
					t.Fatalf("ring %d clicks card %d", i, at)
				}
			}
			if scale == 2 {
				t.Logf("four cards, dark=%v: numbers %.0f pt; rings %.0f pt", dark, float64(textWidth)/scale, float64(w)/scale)
				if out := os.Getenv("MAGPIE_TRAY_PREVIEW"); out != "" {
					for _, mode := range []string{"rings", "numbers"} {
						data := b
						if mode == "numbers" {
							data, _, _ = trayImagePNG(cells, nil, 22, scale, dark, true)
						}
						if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("quota-%s-%s.png", mode, map[bool]string{false: "light", true: "dark"}[dark])), data, 0o644); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}
	}
	// A failure and missing reading have different native symbols from 0%.
	for _, r := range []*trayRing{{Error: true}, {}, {Share: new(float64)}} {
		if b, _, _ := trayImagePNG([]trayCell{{ID: "codex", Letter: "C", Ring: r}}, nil, 22, 2, false, true); len(b) == 0 {
			t.Fatal("state has no image")
		}
	}
}

func TestTrayRingWavesReflectConcurrencyAndReducedMotion(t *testing.T) {
	share := 0.65
	cells := []trayCell{{ID: "codex", Letter: "C", Ring: &trayRing{Share: &share, Active: 1}}}
	first, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.1)
	next, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.4)
	if bytes.Equal(first, next) {
		t.Fatal("an active request has no moving wave")
	}
	cells[0].Ring.Active = 2
	two, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.1)
	if bytes.Equal(first, two) {
		t.Fatal("two requests look like one")
	}
	cells[0].Ring.Active = 5
	five, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.1)
	cells[0].Ring.Active = 7
	seven, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.1)
	if !bytes.Equal(five, seven) {
		t.Fatal("more than five requests should cap the drawn waves")
	}
	stillA, _, _ := trayImagePNGRender(cells, nil, 22, 2, false, true, 0.1, true)
	stillB, _, _ := trayImagePNGRender(cells, nil, 22, 2, false, true, 0.4, true)
	if !bytes.Equal(stillA, stillB) {
		t.Fatal("reduced motion still animates")
	}
	cells[0].Ring.Active = 0
	idleA, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.1)
	idleB, _, _ := trayImagePNGPhase(cells, nil, 22, 2, false, true, 0.4)
	if !bytes.Equal(idleA, idleB) {
		t.Fatal("idle rings still animate")
	}
	if out := os.Getenv("MAGPIE_TRAY_PREVIEW"); out != "" {
		strip := []trayCell{}
		for i, n := range []int{0, 1, 2, 7} {
			strip = append(strip, trayCell{ID: fmt.Sprint(i), Letter: []string{"C", "K", "M", "G"}[i], Ring: &trayRing{Share: &share, Active: n}})
		}
		for frame := 0; frame < 36; frame++ {
			b, _, _ := trayImagePNGPhase(strip, nil, 22, 3, true, true, float64(frame)/36)
			if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("wave-%02d.png", frame)), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
