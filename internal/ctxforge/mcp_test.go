// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package ctxforge

import "testing"

func sampleMCPServer() MCPServer {
	return MCPServer{ID: "fs", Name: "filesystem", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem"}, Enabled: false}
}

func TestAddMCPServer(t *testing.T) {
	f := New(t.TempDir())
	if err := f.AddMCPServer(sampleMCPServer()); err != nil {
		t.Fatalf("valid server rejected: %v", err)
	}
	if err := f.AddMCPServer(sampleMCPServer()); err == nil {
		t.Fatal("expected duplicate server error")
	}
	if err := f.AddMCPServer(MCPServer{ID: "no-cmd"}); err == nil {
		t.Fatal("expected validation error for empty command")
	}
	if len(f.MCPServers) != 1 {
		t.Fatalf("expected 1 server after rejected adds, got %d", len(f.MCPServers))
	}
}

func TestMCPServerLookup(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddMCPServer(sampleMCPServer())
	if f.MCPServer("fs") == nil {
		t.Fatal("expected to find server by id")
	}
	if f.MCPServer("ghost") != nil {
		t.Fatal("expected nil for unknown server")
	}
}

func TestUpdateMCPServer(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddMCPServer(sampleMCPServer())
	edited := sampleMCPServer()
	edited.Name = "Files"
	if err := f.UpdateMCPServer("fs", edited); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
	if f.MCPServer("fs").Name != "Files" {
		t.Fatal("update did not apply")
	}
	if err := f.UpdateMCPServer("missing", edited); err == nil {
		t.Fatal("expected error updating unknown server")
	}
	// Invalid edit (empty command) must roll back, leaving the prior value intact.
	broken := sampleMCPServer()
	broken.Command = ""
	if err := f.UpdateMCPServer("fs", broken); err == nil {
		t.Fatal("expected validation error for empty command")
	}
	if got := f.MCPServer("fs").Command; got == "" {
		t.Fatal("failed update should have rolled back the command")
	}
}

func TestUpdateMCPServerRenameCollisionRollsBack(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddMCPServer(sampleMCPServer())
	other := MCPServer{ID: "git", Name: "git", Command: "uvx", Args: []string{"mcp-server-git"}}
	_ = f.AddMCPServer(other)
	// Rename "git" onto the existing "fs" id → duplicate, must roll back.
	clash := other
	clash.ID = "fs"
	if err := f.UpdateMCPServer("git", clash); err == nil {
		t.Fatal("expected duplicate-id error on rename collision")
	}
	if f.MCPServer("git") == nil {
		t.Fatal("rollback should have restored the original id")
	}
}

func TestToggleMCPServer(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddMCPServer(sampleMCPServer()) // starts disabled
	on, err := f.ToggleMCPServer("fs")
	if err != nil {
		t.Fatalf("toggle failed: %v", err)
	}
	if !on || !f.MCPServer("fs").Enabled {
		t.Fatal("toggle should have enabled the server")
	}
	if _, err := f.ToggleMCPServer("ghost"); err == nil {
		t.Fatal("expected error toggling unknown server")
	}
}

func TestRemoveMCPServer(t *testing.T) {
	f := New(t.TempDir())
	_ = f.AddMCPServer(sampleMCPServer())
	if err := f.RemoveMCPServer("fs"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if len(f.MCPServers) != 0 {
		t.Fatal("server not removed")
	}
	if err := f.RemoveMCPServer("fs"); err == nil {
		t.Fatal("expected error removing unknown server")
	}
}
