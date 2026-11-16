package resourcelogs

import (
	"os"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/testaws"
)

func TestMain(m *testing.M) {
	os.Exit(testaws.Run(m))
}
