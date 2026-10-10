package config

import "testing"

func TestClusterDefaultsToMaster(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Cluster.IsMaster() || cfg.Cluster.NodeType != NodeMaster {
		t.Fatalf("default node type = %q, want master", cfg.Cluster.NodeType)
	}
	if cfg.Cluster.Name() == "" {
		t.Fatal("node name should fall back to the hostname")
	}
}

func TestClusterEnvOverrides(t *testing.T) {
	t.Setenv("LLM_GATEWAY_CLUSTER_NODE_TYPE", " Slave ")
	t.Setenv("LLM_GATEWAY_CLUSTER_NODE_NAME", "gw-slave-1")
	t.Setenv("LLM_GATEWAY_SERVER_TRUSTED_PROXIES", "10.0.0.5, 172.28.0.0/16")
	t.Setenv("LLM_GATEWAY_SERVER_WEB_ROOT", "/app/web")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster.IsMaster() || cfg.Cluster.Name() != "gw-slave-1" {
		t.Fatalf("cluster = %+v, want slave gw-slave-1", cfg.Cluster)
	}
	if got := cfg.Server.TrustedProxies; len(got) != 2 || got[0] != "10.0.0.5" || got[1] != "172.28.0.0/16" {
		t.Fatalf("trusted proxies = %q", got)
	}
	if cfg.Server.WebRoot != "/app/web" {
		t.Fatalf("web root = %q", cfg.Server.WebRoot)
	}
}

func TestClusterRejectsUnknownNodeType(t *testing.T) {
	t.Setenv("LLM_GATEWAY_CLUSTER_NODE_TYPE", "worker")
	if _, err := Load(""); err == nil {
		t.Fatal("expected an error for node_type=worker")
	}
}
