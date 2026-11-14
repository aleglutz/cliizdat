package cellbuf

import "testing"

func TestOverstrike(t *testing.T) {
	for _, tc := range []struct {
		under, over, want rune
		merged            bool
	}{
		{'⠁', '⠈', '⠉', true},
		{'⣿', '⠁', '⣿', true},
		{'▌', '▐', '█', true},
		{'▀', '▄', '█', true},
		{OctGlyph[0x04], OctGlyph[0x08], OctGlyph[0x0C], true},
		{'▘', OctGlyph[0x04], OctGlyph[0x05|0x04], true},
		{'⠁', '▌', '▌', false},
		{'▌', '⠁', '⠁', false},
		{'#', 'E', 'E', false},
		{'▌', 'E', 'E', false},
		{' ', '▌', '▌', false},
	} {
		got, merged := Overstrike(tc.under, tc.over)
		if got != tc.want || merged != tc.merged {
			t.Errorf("Overstrike(%c, %c) = %c (%U), %v; want %c (%U), %v",
				tc.under, tc.over, got, got, merged, tc.want, tc.want, tc.merged)
		}
	}
}
