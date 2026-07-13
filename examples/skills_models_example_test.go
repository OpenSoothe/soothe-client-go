package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_listSkills requests the skills catalog via the blocking
// RequestResponse wrapper and prints skill names.
func Example_listSkills() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// ListSkills sends skills_list and blocks for the response.
	result, err := client.ListSkills(ctx, 15*time.Second)
	if err != nil {
		fmt.Printf("ListSkills error: %v\n", err)
		return
	}

	skills, ok := result["skills"].([]interface{})
	if !ok {
		fmt.Println("No skills in response")
		return
	}
	for _, s := range skills {
		if m, ok := s.(map[string]interface{}); ok {
			fmt.Printf("  - %s: %s\n", m["name"], m["description"])
		}
	}
	// Output:
	//   - research: Research skill
	//   - browser: Browser skill
	//   - code_reviewer: Code review skill
}

// Example_fetchSkillsCatalog uses the package-level helper that returns
// a typed []map[string]interface{} slice for direct iteration.
func Example_fetchSkillsCatalog() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	skills, err := soothe.FetchSkillsCatalog(ctx, client, 15*time.Second)
	if err != nil {
		fmt.Printf("FetchSkillsCatalog error: %v\n", err)
		return
	}
	for _, s := range skills {
		fmt.Printf("  - %v\n", s["name"])
	}
	// Output:
	//   - research
	//   - browser
	//   - code_reviewer
}

// Example_listModels requests the models catalog via the blocking wrapper.
func Example_listModels() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	result, err := client.ListModels(ctx, 15*time.Second)
	if err != nil {
		fmt.Printf("ListModels error: %v\n", err)
		return
	}
	fmt.Printf("Models: %v\n", result)
	// Output:
	// Models: map[models:[map[id:openai:gpt-4o provider:openai] map[id:anthropic:claude-sonnet-4 provider:anthropic]]]
}

// Example_invokeSkill resolves a skill on the daemon host and receives
// its echo response (RFC-400). Uses the blocking InvokeSkill wrapper.
func Example_invokeSkill() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// InvokeSkill resolves a skill by name and passes args as a string.
	// The default timeout is 120s (skills may be long-running).
	result, err := client.InvokeSkill(ctx, "code_reviewer", "review main.go", 120*time.Second)
	if err != nil {
		fmt.Printf("InvokeSkill error: %v\n", err)
		return
	}
	fmt.Printf("Skill result: %v\n", result)
	// Output:
	// Skill result: map[echo:map[skill:code_reviewer status:ok]]
}

// Example_sendSkillsAndModels uses the low-level Send* methods (fire-and-forget)
// for skills_list and models_list, useful when the event reader handles responses.
func Example_sendSkillsAndModels() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// Start the event reader so responses are multiplexed.
	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// Fire-and-forget: send skills_list request.
	if err := client.SendSkillsList(ctx); err != nil {
		fmt.Printf("SendSkillsList error: %v\n", err)
	}

	// Fire-and-forget: send models_list request.
	if err := client.SendModelsList(ctx); err != nil {
		fmt.Printf("SendModelsList error: %v\n", err)
	}

	// Fire-and-forget: send invoke_skill request.
	if err := client.SendInvokeSkill(ctx, "code_reviewer", "review main.go"); err != nil {
		fmt.Printf("SendInvokeSkill error: %v\n", err)
	}

	// Responses arrive on the event channel (correlated by request id).
	go func() {
		for msg := range eventCh {
			if msg == nil {
				return
			}
			// Inspect for response/error frames by request id...
			_ = msg
		}
	}()
	fmt.Println("Skills/models requests sent")
	// Output:
	// Skills/models requests sent
}
