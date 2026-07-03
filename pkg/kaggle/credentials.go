package kaggle

import "errors"

// ErrNoCredentials indicates neither the KAGGLE_USERNAME/KAGGLE_KEY env pair nor
// ~/.kaggle/kaggle.json is available. Its message names both mechanisms so a
// stuck user knows their options.
var ErrNoCredentials = errors.New(
	"kaggle: no credentials — set KAGGLE_USERNAME + KAGGLE_KEY, or install ~/.kaggle/kaggle.json")

// CredentialSource is the PURE auth decision: given whether the env pair is set
// (both values non-empty) and whether the credentials file exists, report whether
// usable Kaggle credentials are present. It never reads the environment or the
// filesystem and never retains the key value — the IO (os.Getenv + os.Stat) lives
// in internal/kagglecli, which passes the results here (ARCH-PURE). Partial env
// (only one of username/key) is not credentials.
func CredentialSource(username, key string, fileExists bool) (present bool, err error) {
	if (username != "" && key != "") || fileExists {
		return true, nil
	}
	return false, ErrNoCredentials
}
