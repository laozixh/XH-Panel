package database

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// backupKeepCount is how many rolling database backups are kept next to the
// live sqlite file.
const backupKeepCount = 5

// BackupSQLite keeps a small rolling copy of the sqlite database in a
// "backup" directory next to it.
//
// It exists because losing the database means losing every bookmark, group and
// account. That already happened once: a deployment that mounted only
// /app/conf kept the database inside the container writable layer, so an image
// upgrade silently replaced it with the demo seed. The rolling copy does not
// save data from a deleted volume, but it does make a bad upgrade, a partial
// write or a corrupted file recoverable.
//
// It is intentionally best effort: any failure is logged and ignored, because
// a broken backup must never prevent the panel from starting.
func BackupSQLite(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return
	}

	backupDir := filepath.Join(filepath.Dir(filePath), "backup")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		log.Println("database backup: cannot create backup dir:", err)
		return
	}

	current, err := fileMD5(filePath)
	if err != nil {
		log.Println("database backup: cannot hash database:", err)
		return
	}

	existing := listBackups(backupDir)
	if len(existing) > 0 {
		if newest, err := fileMD5(filepath.Join(backupDir, existing[0])); err == nil && newest == current {
			// Nothing changed since the last start, no new copy needed.
			return
		}
	}

	name := "database-" + time.Now().Format("20060102-150405") + ".db"
	target := filepath.Join(backupDir, name)
	if err := copyFile(filePath, target); err != nil {
		log.Println("database backup: copy failed:", err)
		return
	}

	// Prune old copies, newest first.
	existing = listBackups(backupDir)
	for i := backupKeepCount; i < len(existing); i++ {
		_ = os.Remove(filepath.Join(backupDir, existing[i]))
	}
}

// listBackups returns backup file names sorted newest first.
func listBackups(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "database-") && strings.HasSuffix(name, ".db") {
			names = append(names, name)
		}
	}
	// Timestamped names sort lexicographically in chronological order.
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names
}

// fileMD5 returns the hex md5 of a file.
func fileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
