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

// https://cloud.google.com/docs/authentication/rest
const (
	GOOGLE_TOKEN_API string = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
)

// Gets an access token from the cloud service we are using, to be used for settint an Authorization hedaer.
func GetAccessTokenFromImplicitServiceAccount(client *http.Client) (string, error) {

	var token GoogleToken

	switch strings.ToLower(os.Getenv("IGOR_CLOUD")) {
	case "google":

		req, err := http.NewRequest("GET", GOOGLE_TOKEN_API, nil)
		if err != nil {
			return "", fmt.Errorf("error creating request for token: %w", err)
		}
		req.Header.Add("Metadata-Flavor", "Google")
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("error doing request for token: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("token request got non OK response: %d %w", resp.StatusCode, err)
		}
		if body, err := io.ReadAll(resp.Body); err != nil {
			return "", fmt.Errorf("could not read the token %w", err)
		} else {
			if err = json.Unmarshal(body, &token); err != nil {
				return "", fmt.Errorf("could not unmarshal the token: %w", err)
			}
		}

		return token.Access_token, nil

	case "":
		return "", nil

	default:
		panic(strings.ToLower(os.Getenv("IGOR_CLOUD") + " cloud not implemented"))

	}
}
