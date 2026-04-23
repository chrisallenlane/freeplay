package igdb

import "testing"

// realisticNames samples the kinds of No-Intro / ROM filenames that
// CleanName and NameVariants encounter in the wild.
var realisticNames = []string{
	"Super Mario Bros.",
	"Super Mario Bros. 3",
	"The Legend of Zelda - A Link to the Past (USA)",
	"Final Fantasy VII [T-En 1.4]",
	"Mega Man X (USA) (Rev 1)",
	"Chrono Trigger (USA) [!]",
	"Sonic the Hedgehog 2 (World)",
	"Street Fighter II Turbo - Hyper Fighting (USA)",
	"Castlevania - Symphony of the Night (USA)",
	"Super Metroid (Japan, USA)",
}

func BenchmarkCleanName(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for _, n := range realisticNames {
			_ = CleanName(n)
		}
	}
}

func BenchmarkNameVariants(b *testing.B) {
	cleaned := make([]string, len(realisticNames))
	for i, n := range realisticNames {
		cleaned[i] = CleanName(n)
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, n := range cleaned {
			_ = NameVariants(n)
		}
	}
}
