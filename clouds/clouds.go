package clouds

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type GoogleToken struct {
	Access_token string
}

const (
	GOOGLE_TOKEN_API string = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
)

func GetAccessTokenFromImplicitServiceAccount(client *http.Client) (string, error) {

	var token GoogleToken

	switch strings.ToLower(os.Getenv("IGOR_CLOUD")) {
	case "google":

		resp, err := client.Get(GOOGLE_TOKEN_API)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("%w", err)
		}
		if body, err := io.ReadAll(resp.Body); err != nil {
			return "", fmt.Errorf("%w", err)
		} else {
			if err = json.Unmarshal(body, &token); err != nil {
				return "", fmt.Errorf("%w", err)
			}
		}

		return token.Access_token, nil

	case "":
		return "", nil

	default:
		panic(strings.ToLower(os.Getenv("IGOR_CLOUD") + " cloud not implemented"))

	}
}
