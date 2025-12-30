package importer

import (
	"os"
)

func ImportCSV(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
}
