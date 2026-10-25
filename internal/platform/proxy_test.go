package platform

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"repodock/internal/domain"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProxyTesterUsesHTTPAndHTTPSConnect(t *testing.T) {
	payload := "{\"resources\":{\"core\":{\"limit\":60,\"remaining\":0}}}"
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, payload) }))
	defer origin.Close()
	for _, secure := range []bool{false, true} {
		t.Run(fmt.Sprint("tls-proxy-", secure), func(t *testing.T) {
			tunnels := make(chan string, 1)
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "CONNECT" {
					t.Error("expected CONNECT", r.Method)
					w.WriteHeader(400)
					return
				}
				upstream, err := net.DialTimeout("tcp", strings.TrimPrefix(origin.URL, "https://"), time.Second)
				if err != nil {
					t.Error(err)
					w.WriteHeader(502)
					return
				}
				client, buffer, err := w.(http.Hijacker).Hijack()
				if err != nil {
					upstream.Close()
					t.Error(err)
					return
				}
				defer client.Close()
				defer upstream.Close()
				fmt.Fprint(buffer, "HTTP/1.1 200 Connection Established\r\n\r\n")
				buffer.Flush()
				tunnels <- r.Host
				go func() { io.Copy(upstream, buffer); upstream.Close() }()
				io.Copy(client, upstream)
			})
			var server *httptest.Server
			if secure {
				server = httptest.NewTLSServer(handler)
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			transport := origin.Client().Transport.(*http.Transport)
			if err := (ProxyTester{Endpoint: origin.URL, Transport: transport}).Test(ctx, domain.ProxyConfig{URL: server.URL}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-tunnels:
			default:
				t.Fatal("TLS request bypassed proxy")
			}
		})
	}
}

func TestProxyTesterUsesSOCKS5(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	hits := make(chan string, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		connection.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(connection)
		header := make([]byte, 2)
		if _, err = io.ReadFull(reader, header); err != nil {
			return
		}
		if header[0] != 5 {
			return
		}
		if _, err = io.CopyN(io.Discard, reader, int64(header[1])); err != nil {
			return
		}
		connection.Write([]byte{5, 0})
		command := make([]byte, 4)
		if _, err = io.ReadFull(reader, command); err != nil || command[1] != 1 {
			return
		}
		length := 0
		switch command[3] {
		case 1:
			length = 4
		case 4:
			length = 16
		case 3:
			b, e := reader.ReadByte()
			if e != nil {
				return
			}
			length = int(b)
		default:
			return
		}
		if _, err = io.CopyN(io.Discard, reader, int64(length+2)); err != nil {
			return
		}
		connection.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
		request, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		hits <- request.Host
		payload := "{\"resources\":{\"core\":{\"limit\":60,\"remaining\":0}}}"
		fmt.Fprintf(connection, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(payload), payload)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := (ProxyTester{Endpoint: "http://api.github.test/"}).Test(ctx, domain.ProxyConfig{URL: "socks5://" + listener.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	select {
	case host := <-hits:
		if host != "api.github.test" {
			t.Fatal(host)
		}
	default:
		t.Fatal("SOCKS proxy received no request")
	}
}

func envValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		k, v, _ := strings.Cut(env[i], "=")
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func TestProxyEnvironmentAndGitConfigurationStayPerProcess(t *testing.T) {
	inherited := []string{"HTTP_PROXY=http://old:80", "https_proxy=http://old:81", "ALL_PROXY=socks5://old:82", "NO_PROXY=*", "no_proxy=github.com", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.proxy", "GIT_CONFIG_VALUE_0=old", "PATH=path"}
	cmd := Command{Path: "git", Args: []string{"ls-remote", "https://github.com/you/repo.git"}, Env: []string{"NO_PROXY=*", "GIT_TERMINAL_PROMPT=0"}}
	r := ExecRunner{Proxy: domain.ProxyConfig{Enabled: true, URL: domain.DefaultProxyURL}, GitPath: "git"}
	args, env, err := r.prepare(cmd, inherited)
	if err != nil || args[0] != "-c" || envValue(env, "NO_PROXY") != "" || envValue(env, "GIT_CONFIG_COUNT") != "4" {
		t.Fatal(args, env, err)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		if !slices.Contains(env, key+"="+domain.DefaultProxyURL) {
			t.Fatal(key, env)
		}
	}
	if cmd.Args[0] != "ls-remote" || inherited[0] != "HTTP_PROXY=http://old:80" {
		t.Fatal("mutated caller data")
	}
	offArgs, offEnv, err := (ExecRunner{}).prepare(cmd, inherited)
	if err != nil || !slices.Equal(offArgs, cmd.Args) || envValue(offEnv, "NO_PROXY") != "*" || envValue(offEnv, "HTTPS_PROXY") != "http://old:81" {
		t.Fatal(offArgs, offEnv, err)
	}
	if envValue(offEnv, "GIT_CONFIG_COUNT") != "" {
		t.Fatal("inherited Git injection survived")
	}
}

func TestProxyTesterRequestsAPIThroughDraftAndRejectsPortal(t *testing.T) {
	for _, kind := range []string{"success", "portal", "http-error"} {
		t.Run(kind, func(t *testing.T) {
			hits := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits <- r.URL.String()
				switch kind {
				case "success":
					fmt.Fprint(w, "{\"resources\":{\"core\":{\"limit\":60,\"remaining\":0}}}")
				case "portal":
					fmt.Fprint(w, "login portal")
				case "http-error":
					w.WriteHeader(502)
				}
			}))
			defer server.Close()
			err := (ProxyTester{Endpoint: "http://api.github.test/"}).Test(context.Background(), domain.ProxyConfig{URL: server.URL})
			if (kind == "success") != (err == nil) {
				t.Fatal(err)
			}
			select {
			case path := <-hits:
				if path != "http://api.github.test/" {
					t.Fatal(path)
				}
			default:
				t.Fatal("proxy did not receive an HTTP request")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()
	if err := (ProxyTester{Endpoint: "http://api.github.test/"}).Test(context.Background(), domain.ProxyConfig{URL: endpoint}); err == nil {
		t.Fatal("failed proxy fell back to direct")
	}
}

func TestProxyChildEnvironmentAndGitChildInheritance(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	r := ExecRunner{Proxy: domain.ProxyConfig{Enabled: true, URL: domain.DefaultProxyURL}, GitPath: git}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	data, err := r.Run(ctx, Command{Path: os.Args[0], Args: []string{"-test.run=^TestProxyChildHelper$"}, Env: []string{"REPODOCK_PROXY_HELPER=1", "REPODOCK_PROXY_GIT=" + git, "NO_PROXY=*"}}, nil)
	if err != nil || !strings.Contains(string(data), "inherited:"+domain.DefaultProxyURL) {
		t.Fatal(string(data), err)
	}
}

func TestProxyChildHelper(t *testing.T) {
	if os.Getenv("REPODOCK_PROXY_HELPER") != "1" {
		return
	}
	if os.Getenv("HTTPS_PROXY") != domain.DefaultProxyURL || os.Getenv("NO_PROXY") != "" {
		os.Exit(2)
	}
	cmd := exec.Command(os.Getenv("REPODOCK_PROXY_GIT"), "config", "--get", "http.proxy")
	data, err := cmd.Output()
	if err != nil {
		os.Exit(3)
	}
	fmt.Print("inherited:" + strings.TrimSpace(string(data)))
	os.Exit(0)
}

// Real installed gh/Git processes send CONNECT to a local rejecting proxy.
// No TLS handshake, credentials, GitHub request, repository, or global config
// changes are involved.
func TestInstalledGitAndGHUseSelectedProxy(t *testing.T) {
	for _, tool := range []string{"git", "gh"} {
		t.Run(tool, func(t *testing.T) {
			path, err := exec.LookPath(tool)
			if err != nil {
				t.Skip(tool + " is not installed")
			}
			var mu sync.Mutex
			hits := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				hits = append(hits, r.Method+" "+r.Host)
				mu.Unlock()
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer server.Close()
			git, _ := exec.LookPath("git")
			cmd := Command{Path: path, Env: []string{"GH_TOKEN=fixture-token", "GH_CONFIG_DIR=" + t.TempDir(), "GH_HOST=github.com", "GIT_TERMINAL_PROMPT=0", "NO_PROXY=*"}}
			if tool == "git" {
				cmd.Args = []string{"ls-remote", "https://github.com/repodock-fixture/no-network.git"}
			} else {
				cmd.Args = []string{"api", "--hostname", "github.com", "user"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_, err = (ExecRunner{GitPath: git, Proxy: domain.ProxyConfig{Enabled: true, URL: server.URL}}).Run(ctx, cmd, nil)
			mu.Lock()
			defer mu.Unlock()
			if err == nil || len(hits) == 0 || !strings.Contains(hits[0], "CONNECT ") {
				t.Fatal(hits, err)
			}
		})
	}
}
