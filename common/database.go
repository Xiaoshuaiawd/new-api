package common

const (
	DatabaseTypeMySQL      = "mysql"
	DatabaseTypeSQLite     = "sqlite"
	DatabaseTypePostgreSQL = "postgres"
)

var UsingSQLite = false
var UsingPostgreSQL = false
var LogSqlType = DatabaseTypeSQLite // Default to SQLite for logging SQL queries
var UsingMySQL = false
var UsingClickHouse = false

var SQLitePath = "one-api.db?_busy_timeout=30000"

// MES Database variables for chat history storage
var MesSqlType = DatabaseTypeSQLite // Default to SQLite for MES (Message/Conversation history) database
var UsingMESMySQL = false
var UsingMESPostgreSQL = false
var UsingMESSQLite = false
var MESEnabled = false        // Whether MES database is enabled (MES_SQL_DSN is set)
var MESDailyPartition = false // Whether to use daily partitioning for MES tables
