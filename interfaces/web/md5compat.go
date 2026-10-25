package web

import (
	"crypto/md5"
	"encoding/hex"
)

// md5hex replicates PHP md5() used by the legacy SalesERP password scheme.
// Kept ONLY for verifying migrated users; new passwords use bcrypt.
func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
