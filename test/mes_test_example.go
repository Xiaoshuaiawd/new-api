// 这是一个演示 MES 功能的示例测试文件
// 要运行此测试，您需要先设置数据库环境
package test

import (
	"fmt"
	"one-api/common"
	"one-api/model"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// 这不是真正的测试，而是如何测试 MES 功能的示例
func ExampleMESUsage() {
	// 设置测试用的环境变量
	os.Setenv("MES_SQL_DSN", "root:123456@tcp(localhost:3306)/test_mes")
	os.Setenv("MES_DAILY_PARTITION", "false")

	// Initialize common variables
	common.InitEnv()

	// Initialize databases
	err := model.InitDB()
	if err != nil {
		fmt.Printf("Failed to init main DB: %v\n", err)
		return
	}

	err = model.InitMESDB()
	if err != nil {
		fmt.Printf("Failed to init MES DB: %v\n", err)
		return
	}

	// Create a test Gin context
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	// Example 1: Save a conversation
	conversationId := "test_conv_123"
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hello, how are you?",
		},
	}

	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello! I'm doing well, thank you for asking.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 15,
			"total_tokens":      25,
		},
	}

	mesHelper := model.GetMESHelper()
	err = mesHelper.SaveChatCompletion(c, conversationId, messages, response, "gpt-3.5-turbo", 1, 1, 1)
	if err != nil {
		fmt.Printf("Failed to save chat completion: %v\n", err)
		return
	}

	fmt.Println("✓ Chat completion saved successfully")

	// Example 2: Retrieve conversation history
	retrievedMessages, err := mesHelper.GetConversationMessages(conversationId, 10)
	if err != nil {
		fmt.Printf("Failed to get conversation messages: %v\n", err)
		return
	}

	fmt.Printf("✓ Retrieved %d messages from conversation\n", len(retrievedMessages))

	// Example 3: Save an error conversation
	errorMessages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "This message caused an error",
		},
	}

	err = mesHelper.SaveErrorConversation(c, "error_conv_123", errorMessages, 400, "Content filter triggered", "gpt-4", 1, 1, 1)
	if err != nil {
		fmt.Printf("Failed to save error conversation: %v\n", err)
		return
	}

	fmt.Println("✓ Error conversation saved successfully")

	// Example 4: Get user statistics
	stats, err := mesHelper.GetConversationStats(1)
	if err != nil {
		fmt.Printf("Failed to get conversation stats: %v\n", err)
		return
	}

	fmt.Printf("✓ User stats: %+v\n", stats)

	// Example 5: Delete conversation
	err = mesHelper.DeleteUserConversation(1, conversationId)
	if err != nil {
		fmt.Printf("Failed to delete conversation: %v\n", err)
		return
	}

	fmt.Println("✓ Conversation deleted successfully")

	fmt.Println("\n🎉 All MES functionality tests completed successfully!")
}

// Example function to test daily partitioning
func ExampleMESDailyPartitioning() {
	// Set up environment variables for daily partitioning test
	os.Setenv("MES_SQL_DSN", "root:123456@tcp(localhost:3306)/test_mes_partition")
	os.Setenv("MES_DAILY_PARTITION", "true")

	// Initialize
	common.InitEnv()
	err := model.InitDB()
	if err != nil {
		fmt.Printf("Failed to init main DB: %v\n", err)
		return
	}

	err = model.InitMESDB()
	if err != nil {
		fmt.Printf("Failed to init MES DB: %v\n", err)
		return
	}

	// Test saving conversation with daily partitioning
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)

	mesHelper := model.GetMESHelper()
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Test message for daily partitioning",
		},
	}

	response := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "This is a test response for daily partitioning",
				},
				"finish_reason": "stop",
			},
		},
	}

	err = mesHelper.SaveChatCompletion(c, "partition_test_conv", messages, response, "gpt-3.5-turbo", 1, 1, 1)
	if err != nil {
		fmt.Printf("Failed to save chat completion with partitioning: %v\n", err)
		return
	}

	fmt.Println("✓ Chat completion saved with daily partitioning")

	// Retrieve messages across partitions
	retrievedMessages, err := mesHelper.GetConversationMessages("partition_test_conv", 10)
	if err != nil {
		fmt.Printf("Failed to get messages from partitioned tables: %v\n", err)
		return
	}

	fmt.Printf("✓ Retrieved %d messages from partitioned tables\n", len(retrievedMessages))

	fmt.Println("\n🎉 Daily partitioning test completed successfully!")
}

func TestMESFunctionality(t *testing.T) {
	// This is a placeholder test - actual tests would require database setup
	t.Log("MES functionality test placeholder")
	t.Log("To run actual tests, set up test databases and run ExampleMESUsage()")
	t.Log("Example databases: test_mes, test_mes_partition")
}
