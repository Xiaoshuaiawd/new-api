package model

import (
	"fmt"
	"log"
	"one-api/common"
	"one-api/constant"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var commonGroupCol string
var commonKeyCol string
var commonTrueVal string
var commonFalseVal string

var logKeyCol string
var logGroupCol string

func initCol() {
	// init common column names
	if common.UsingPostgreSQL {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	if os.Getenv("LOG_SQL_DSN") != "" {
		switch common.LogSqlType {
		case common.DatabaseTypePostgreSQL:
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		default:
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	} else {
		// LOG_SQL_DSN 为空时，日志数据库与主数据库相同
		if common.UsingPostgreSQL {
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		} else {
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	}
	// log sql type and database type
	//common.SysLog("Using Log SQL Type: " + common.LogSqlType)
}

var DB *gorm.DB

var LOG_DB *gorm.DB

// dropIndexIfExists drops a MySQL index only if it exists to avoid noisy 1091 errors
func dropIndexIfExists(tableName string, indexName string) {
    if !common.UsingMySQL {
        return
    }
    var count int64
    // Check index existence via information_schema
    err := DB.Raw(
        "SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?",
        tableName, indexName,
    ).Scan(&count).Error
    if err == nil && count > 0 {
        _ = DB.Exec("ALTER TABLE " + tableName + " DROP INDEX " + indexName + ";").Error
    }
}

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func chooseDB(envName string, isLog bool, isMES bool) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			var dbType string
			if isMES {
				dbType = "MES PostgreSQL"
				common.UsingMESPostgreSQL = true
				common.MesSqlType = common.DatabaseTypePostgreSQL
			} else if isLog {
				dbType = "Log PostgreSQL"
				common.LogSqlType = common.DatabaseTypePostgreSQL
			} else {
				dbType = "PostgreSQL"
				common.UsingPostgreSQL = true
			}
			common.SysLog("using " + dbType + " as database")

			return gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		if strings.HasPrefix(dsn, "local") {
			var dbType string
			if isMES {
				dbType = "MES SQLite"
				common.UsingMESSQLite = true
				common.MesSqlType = common.DatabaseTypeSQLite
			} else if isLog {
				dbType = "Log SQLite"
				common.LogSqlType = common.DatabaseTypeSQLite
			} else {
				dbType = "SQLite"
				common.UsingSQLite = true
			}
			common.SysLog(envName + " not set, using " + dbType + " as database")

			return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		// Use MySQL
		var dbType string
		if isMES {
			dbType = "MES MySQL"
			common.UsingMESMySQL = true
			common.MesSqlType = common.DatabaseTypeMySQL
		} else if isLog {
			dbType = "Log MySQL"
			common.LogSqlType = common.DatabaseTypeMySQL
		} else {
			dbType = "MySQL"
			common.UsingMySQL = true
		}
		common.SysLog("using " + dbType + " as database")

		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}

		return gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	}
	// Use SQLite
	var dbType string
	if isMES {
		dbType = "MES SQLite"
		common.UsingMESSQLite = true
		common.MesSqlType = common.DatabaseTypeSQLite
	} else if isLog {
		dbType = "Log SQLite"
		common.LogSqlType = common.DatabaseTypeSQLite
	} else {
		dbType = "SQLite"
		common.UsingSQLite = true
	}
	common.SysLog(envName + " not set, using " + dbType + " as database")

	return gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN", false, false)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		return
	}
	db, err := chooseDB("LOG_SQL_DSN", true, false)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

// InitMESDB initializes the MES (Message/Conversation history) database
func InitMESDB() (err error) {
	if os.Getenv("MES_SQL_DSN") == "" {
		common.SysLog("MES_SQL_DSN not set, chat history will be stored in main database")
		MES_DB = DB
		common.MESEnabled = false
		return nil
	}

	common.MESEnabled = true
	common.MESDailyPartition = common.GetEnvOrDefaultBool("MES_DAILY_PARTITION", false)
	if common.MESDailyPartition {
		common.SysLog("MES daily partitioning enabled")
	}

	// Try to create database first if it's MySQL
	err = createMESDatabaseIfNeeded()
	if err != nil {
		common.SysError("failed to create MES database: " + err.Error())
		// Continue anyway - the database might already exist
	}

	db, err := chooseDB("MES_SQL_DSN", false, true)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		MES_DB = db
		sqlDB, err := MES_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("MES database migration started")
		err = migrateMESDB()
		if err != nil {
			return err
		}
		common.SysLog("MES database initialized successfully")
		return nil
	} else {
		common.FatalLog("failed to initialize MES database: " + err.Error())
	}
	return err
}

// createMESDatabaseIfNeeded creates the MES database if it doesn't exist (MySQL only)
func createMESDatabaseIfNeeded() error {
	dsn := os.Getenv("MES_SQL_DSN")
	if dsn == "" {
		return nil
	}

	// Only handle MySQL automatic database creation
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		common.SysLog("PostgreSQL detected for MES database. Please ensure the database exists manually.")
		return nil
	}

	if strings.HasPrefix(dsn, "local") {
		// SQLite - no need to create database
		return nil
	}

	// MySQL case - extract database name and create if needed
	return createMySQLDatabaseIfNeeded(dsn, "MES")
}

// createMySQLDatabaseIfNeeded creates MySQL database if it doesn't exist
func createMySQLDatabaseIfNeeded(dsn string, dbType string) error {
	// Parse DSN to extract database name
	// Format: username:password@tcp(host:port)/database_name
	parts := strings.Split(dsn, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid DSN format")
	}

	dbName := parts[len(parts)-1]
	// Remove query parameters if any
	if idx := strings.Index(dbName, "?"); idx != -1 {
		dbName = dbName[:idx]
	}

	// Create DSN without database name to connect to MySQL server
	baseDSN := strings.Join(parts[:len(parts)-1], "/") + "/"

	// Add parseTime if not present
	if !strings.Contains(baseDSN, "parseTime") {
		if strings.Contains(baseDSN, "?") {
			baseDSN += "&parseTime=true"
		} else {
			baseDSN += "?parseTime=true"
		}
	}

	// Connect to MySQL server
	db, err := gorm.Open(mysql.Open(baseDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL server: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	// Check if database exists
	var count int64
	err = db.Raw("SELECT COUNT(*) FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?", dbName).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check if database exists: %v", err)
	}

	if count == 0 {
		// Create database
		createSQL := fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName)
		err = db.Exec(createSQL).Error
		if err != nil {
			return fmt.Errorf("failed to create database %s: %v", dbName, err)
		}
		common.SysLog(fmt.Sprintf("%s database '%s' created successfully", dbType, dbName))
	} else {
		common.SysLog(fmt.Sprintf("%s database '%s' already exists", dbType, dbName))
	}

	return nil
}

// migrateMESDB performs database migration for MES tables
func migrateMESDB() error {
	if common.MESDailyPartition {
		// For daily partitioning, we only create base tables for reference
		// Actual tables will be created on demand
		common.SysLog("MES daily partitioning enabled - tables will be created on demand")
		return nil
	}

	// Create normal tables
	err := MES_DB.AutoMigrate(&ConversationHistory{}, &ErrorConversationHistory{})
	if err != nil {
		return fmt.Errorf("failed to migrate MES database: %v", err)
	}

	common.SysLog("MES database migration completed")
	return nil
}

func migrateDB() error {
	// 修复旧版本留下的唯一索引，允许软删除后重新插入同名记录
	dropIndexIfExists("models", "uk_model_name")
	dropIndexIfExists("vendors", "uk_vendor_name")
	if !common.UsingPostgreSQL {
		return migrateDBFast()
	}
	err := DB.AutoMigrate(
		&Channel{},
		&Token{},
		&User{},
		&Option{},
		&Redemption{},
		&Ability{},
		&Log{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
	)
	if err != nil {
		return err
	}
	return nil
}

func migrateDBFast() error {
	// 修复旧版本留下的唯一索引，允许软删除后重新插入同名记录
	dropIndexIfExists("models", "uk_model_name")
	dropIndexIfExists("vendors", "uk_vendor_name")

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&Ability{}, "Ability"},
		{&Log{}, "Log"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
        {&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	common.SysLog("database migrated")
	return nil
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return err
	}
	return nil
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
