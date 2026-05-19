package main
import (
"fmt"
"io/ioutil"
"net/http"
"os"
)
func main() {
token, _ := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
req, _ := http.NewRequest("GET", "https://kubernetes.default.svc:443/api/v1/pods", nil)
req.Header.Set("Authorization", "Bearer "+string(token))
client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
resp, err := client.Do(req)
fmt.Println(resp.Status, err)
}
