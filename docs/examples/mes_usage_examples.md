# MES (Message/Conversation History) Usage Examples

This document provides examples of how to use the MES (Message/Conversation History) functionality in New API.

## Environment Variables Configuration

### Basic Configuration

```bash
# Main database
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# MES database for chat history
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages
```

### Advanced Configuration with Daily Partitioning

```bash
# Main database
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# MES database with daily partitioning enabled
MES_SQL_DSN=root:123456@tcp(localhost:3306)/oneapi_messages
MES_DAILY_PARTITION=true
```

### PostgreSQL Example

```bash
# Main database (MySQL)
SQL_DSN=root:123456@tcp(localhost:3306)/oneapi

# MES database (PostgreSQL)
MES_SQL_DSN=postgres://user:password@localhost:5432/oneapi_messages
```

### Docker Compose Example

```yaml
version: '3.4'

services:
  new-api:
    image: calciumion/new-api:latest
    container_name: new-api
    restart: always
    ports:
      - "3000:3000"
    environment:
      - SQL_DSN=root:123456@tcp(mysql:3306)/new-api
      - MES_SQL_DSN=root:123456@tcp(mysql:3306)/new-api-mes
      - MES_DAILY_PARTITION=true
      - REDIS_CONN_STRING=redis://redis
      - TZ=Asia/Shanghai
    depends_on:
      - redis
      - mysql

  mysql:
    image: mysql:8.2
    container_name: mysql
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: 123456
      MYSQL_DATABASE: new-api
    volumes:
      - mysql_data:/var/lib/mysql
      # Initialize both databases
      - ./init-mes-db.sql:/docker-entrypoint-initdb.d/init-mes-db.sql

volumes:
  mysql_data:
```

### Database Initialization Script (init-mes-db.sql)

```sql
-- Create MES database if it doesn't exist
CREATE DATABASE IF NOT EXISTS `new-api-mes` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

## Code Usage Examples

### Example 1: Basic Chat Completion with History Saving

```go
package main

import (
    "one-api/model"
    "github.com/gin-gonic/gin"
    "fmt"
)

func handleChatCompletion(c *gin.Context) {
    // Your existing chat completion logic here...
    
    // Extract conversation details
    conversationId := "conv_123456789"
    userId := 1
    tokenId := 1
    channelId := 1
    modelName := "gpt-3.5-turbo"
    
    // Input messages from user
    messages := []map[string]interface{}{
        {
            "role": "user",
            "content": "Hello, how are you?",
        },
    }
    
    // Simulated response from AI
    response := map[string]interface{}{
        "choices": []interface{}{
            map[string]interface{}{
                "message": map[string]interface{}{
                    "role": "assistant",
                    "content": "Hello! I'm doing well, thank you for asking. How can I help you today?",
                },
                "finish_reason": "stop",
            },
        },
        "usage": map[string]interface{}{
            "prompt_tokens": 15,
            "completion_tokens": 25,
            "total_tokens": 40,
        },
    }
    
    // Save to MES database
    mesHelper := model.GetMESHelper()
    err := mesHelper.SaveChatCompletion(c, conversationId, messages, response, modelName, userId, tokenId, channelId)
    if err != nil {
        fmt.Printf("Failed to save chat history: %v\n", err)
    }
}
```

### Example 2: Error Handling with MES

```go
func handleChatCompletionWithError(c *gin.Context) {
    conversationId := "conv_error_123"
    userId := 1
    tokenId := 1
    channelId := 1
    modelName := "gpt-4"
    
    messages := []map[string]interface{}{
        {
            "role": "user",
            "content": "This is a message that will cause an error",
        },
    }
    
    // Simulate an error occurred
    errorCode := 400
    errorMessage := "Invalid request: content filter triggered"
    
    // Save error to MES database
    mesHelper := model.GetMESHelper()
    err := mesHelper.SaveErrorConversation(c, conversationId, messages, errorCode, errorMessage, modelName, userId, tokenId, channelId)
    if err != nil {
        fmt.Printf("Failed to save error conversation: %v\n", err)
    }
}
```

### Example 3: Retrieving Conversation History

```go
func getConversationHistory(c *gin.Context) {
    conversationId := c.Param("conversation_id")
    limit := 50 // Get last 50 messages
    
    mesHelper := model.GetMESHelper()
    messages, err := mesHelper.GetConversationMessages(conversationId, limit)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"messages": messages})
}
```

### Example 4: User Conversation Management

```go
func getUserConversations(c *gin.Context) {
    userId := c.GetInt("user_id") // From authentication middleware
    limit := 20
    offset := 0
    
    mesHelper := model.GetMESHelper()
    conversations, err := mesHelper.GetUserConversations(userId, limit, offset)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"conversations": conversations})
}

func deleteUserConversation(c *gin.Context) {
    userId := c.GetInt("user_id")
    conversationId := c.Param("conversation_id")
    
    mesHelper := model.GetMESHelper()
    err := mesHelper.DeleteUserConversation(userId, conversationId)
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"message": "Conversation deleted successfully"})
}
```

### Example 5: Conversation Statistics

```go
func getUserStats(c *gin.Context) {
    userId := c.GetInt("user_id")
    
    mesHelper := model.GetMESHelper()
    stats, err := mesHelper.GetConversationStats(userId)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, stats)
}
```

## API Endpoints Example

Here's how you might integrate MES into your API routes:

```go
// api-router.go
func SetupMESRoutes(router *gin.Engine) {
    api := router.Group("/api/v1")
    api.Use(authMiddleware()) // Your authentication middleware
    
    // Conversation management
    api.GET("/conversations", getUserConversations)
    api.GET("/conversations/:conversation_id", getConversationHistory)
    api.DELETE("/conversations/:conversation_id", deleteUserConversation)
    
    // Statistics
    api.GET("/stats/conversations", getUserStats)
}
```

## Database Schema

### With Daily Partitioning Disabled

When `MES_DAILY_PARTITION=false` or not set, the following tables are created:

- `conversation_histories`
- `error_conversation_histories`

### With Daily Partitioning Enabled

When `MES_DAILY_PARTITION=true`, tables are created dynamically:

- `conversation_histories_2025_01_15`
- `conversation_histories_2025_01_16`
- `error_conversation_histories_2025_01_15`
- `error_conversation_histories_2025_01_16`

## Performance Considerations

### Daily Partitioning Benefits

1. **Improved Query Performance**: Smaller tables mean faster queries
2. **Easy Archival**: Old tables can be easily archived or deleted
3. **Maintenance**: Index rebuilding and maintenance operations are faster

### Daily Partitioning Considerations

1. **Query Complexity**: Cross-date queries require searching multiple tables
2. **Storage**: Each day creates new tables, monitor disk usage
3. **Backup**: Consider backing up old partitions separately

### Recommended Settings

For high-volume applications:
```bash
MES_DAILY_PARTITION=true
SQL_MAX_IDLE_CONNS=50
SQL_MAX_OPEN_CONNS=500
SQL_MAX_LIFETIME=300
```

For low-volume applications:
```bash
MES_DAILY_PARTITION=false
SQL_MAX_IDLE_CONNS=10
SQL_MAX_OPEN_CONNS=100
SQL_MAX_LIFETIME=60
```

## Monitoring and Maintenance

### Monitor MES Database Size

```sql
-- MySQL
SELECT 
    table_schema AS 'Database',
    table_name AS 'Table',
    ROUND(((data_length + index_length) / 1024 / 1024), 2) AS 'Size (MB)'
FROM information_schema.TABLES 
WHERE table_schema = 'oneapi_messages'
ORDER BY (data_length + index_length) DESC;
```

### Clean Up Old Partitions

```sql
-- Drop tables older than 30 days (be careful!)
DROP TABLE IF EXISTS conversation_histories_2024_12_15;
DROP TABLE IF EXISTS error_conversation_histories_2024_12_15;
```

### Archive Old Data

```sql
-- Archive old data to another database/table
CREATE TABLE archive_conversation_histories_2024_12 
SELECT * FROM conversation_histories_2024_12_01;
-- ... then drop the original table
```