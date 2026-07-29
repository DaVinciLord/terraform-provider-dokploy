package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVolumeBackupResource_Postgres(t *testing.T) {
	host := os.Getenv("DOKPLOY_HOST")
	apiKey := os.Getenv("DOKPLOY_API_KEY")

	if host == "" || apiKey == "" {
		t.Skip("DOKPLOY_HOST and DOKPLOY_API_KEY must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccVolumeBackupResourceConfig_Postgres("test-volbk-project", "test-volbk-env", "test-volbk-pg", "testvbpg", "testdb", "testuser", "test-volbk-dest", "pg-vol-backup", "0 3 * * *"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "name", "pg-vol-backup"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "volume_name", "postgres_data"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "prefix", "pg-vol"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "cron_expression", "0 3 * * *"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "service_type", "postgres"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "enabled", "true"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "turn_off", "false"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "keep_latest_count", "5"),
					resource.TestCheckResourceAttrSet("dokploy_volume_backup.test", "id"),
					resource.TestCheckResourceAttrSet("dokploy_volume_backup.test", "service_id"),
					resource.TestCheckResourceAttrSet("dokploy_volume_backup.test", "destination_id"),
					resource.TestCheckResourceAttrSet("dokploy_volume_backup.test", "created_at"),
				),
			},
			// Update and Read testing
			{
				Config: testAccVolumeBackupResourceConfig_PostgresUpdated("test-volbk-project", "test-volbk-env", "test-volbk-pg", "testvbpg", "testdb", "testuser", "test-volbk-dest", "pg-vol-backup-updated", "0 4 * * *"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "name", "pg-vol-backup-updated"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "cron_expression", "0 4 * * *"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "enabled", "false"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test", "keep_latest_count", "10"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "dokploy_volume_backup.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccVolumeBackupResource_Redis(t *testing.T) {
	host := os.Getenv("DOKPLOY_HOST")
	apiKey := os.Getenv("DOKPLOY_API_KEY")

	if host == "" || apiKey == "" {
		t.Skip("DOKPLOY_HOST and DOKPLOY_API_KEY must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccVolumeBackupResourceConfig_Redis("test-volbk-redis-project", "test-volbk-redis-env", "test-volbk-redis", "testvbredis", "test-volbk-redis-dest"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("dokploy_volume_backup.test_redis", "name", "redis-vol-backup"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test_redis", "volume_name", "redis_data"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test_redis", "service_type", "redis"),
					resource.TestCheckResourceAttr("dokploy_volume_backup.test_redis", "enabled", "true"),
					resource.TestCheckResourceAttrSet("dokploy_volume_backup.test_redis", "id"),
				),
			},
		},
	})
}

func TestAccVolumeBackupsDataSource(t *testing.T) {
	host := os.Getenv("DOKPLOY_HOST")
	apiKey := os.Getenv("DOKPLOY_API_KEY")

	if host == "" || apiKey == "" {
		t.Skip("DOKPLOY_HOST and DOKPLOY_API_KEY must be set for acceptance tests")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVolumeBackupsDataSourceConfig("test-volbk-ds-project", "test-volbk-ds-env", "test-volbk-ds-pg", "testvbdspg", "testdb", "testuser", "test-volbk-ds-dest"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.dokploy_volume_backups.test", "volume_backups.#"),
				),
			},
		},
	})
}

func testAccVolumeBackupResourceConfig_Postgres(projectName, envName, pgName, appName, dbName, dbUser, destName, backupName, cron string) string {
	return fmt.Sprintf(`
provider "dokploy" {
  host    = "%s"
  api_key = "%s"
}

resource "dokploy_project" "test" {
  name        = "%s"
  description = "Test project for volume backup tests"
}

resource "dokploy_environment" "test" {
  project_id = dokploy_project.test.id
  name       = "%s"
}

resource "dokploy_postgres" "test" {
  name              = "%s"
  app_name_prefix   = "%s"
  database_name     = "%s"
  database_user     = "%s"
  database_password = "test_password_123"
  environment_id    = dokploy_environment.test.id
}

resource "dokploy_destination" "test" {
  name              = "%s"
  storage_provider  = "s3"
  access_key        = "test-access-key"
  secret_access_key = "test-secret-key"
  bucket            = "test-vol-backups"
  region            = "us-east-1"
  endpoint          = "https://s3.amazonaws.com"
}

resource "dokploy_volume_backup" "test" {
  name            = "%s"
  volume_name     = "postgres_data"
  prefix          = "pg-vol"
  destination_id  = dokploy_destination.test.id
  cron_expression = "%s"
  service_type    = "postgres"
  service_id      = dokploy_postgres.test.id
  app_name        = dokploy_postgres.test.app_name
  keep_latest_count = 5
  enabled         = true
}
`, os.Getenv("DOKPLOY_HOST"), os.Getenv("DOKPLOY_API_KEY"), projectName, envName, pgName, appName, dbName, dbUser, destName, backupName, cron)
}

func testAccVolumeBackupResourceConfig_PostgresUpdated(projectName, envName, pgName, appName, dbName, dbUser, destName, backupName, cron string) string {
	return fmt.Sprintf(`
provider "dokploy" {
  host    = "%s"
  api_key = "%s"
}

resource "dokploy_project" "test" {
  name        = "%s"
  description = "Test project for volume backup tests"
}

resource "dokploy_environment" "test" {
  project_id = dokploy_project.test.id
  name       = "%s"
}

resource "dokploy_postgres" "test" {
  name              = "%s"
  app_name_prefix   = "%s"
  database_name     = "%s"
  database_user     = "%s"
  database_password = "test_password_123"
  environment_id    = dokploy_environment.test.id
}

resource "dokploy_destination" "test" {
  name              = "%s"
  storage_provider  = "s3"
  access_key        = "test-access-key"
  secret_access_key = "test-secret-key"
  bucket            = "test-vol-backups"
  region            = "us-east-1"
  endpoint          = "https://s3.amazonaws.com"
}

resource "dokploy_volume_backup" "test" {
  name            = "%s"
  volume_name     = "postgres_data"
  prefix          = "pg-vol-updated"
  destination_id  = dokploy_destination.test.id
  cron_expression = "%s"
  service_type    = "postgres"
  service_id      = dokploy_postgres.test.id
  app_name        = dokploy_postgres.test.app_name
  keep_latest_count = 10
  enabled         = false
}
`, os.Getenv("DOKPLOY_HOST"), os.Getenv("DOKPLOY_API_KEY"), projectName, envName, pgName, appName, dbName, dbUser, destName, backupName, cron)
}

func testAccVolumeBackupResourceConfig_Redis(projectName, envName, redisName, appNamePrefix, destName string) string {
	return fmt.Sprintf(`
provider "dokploy" {
  host    = "%s"
  api_key = "%s"
}

resource "dokploy_project" "test_redis" {
  name        = "%s"
  description = "Test project for redis volume backup tests"
}

resource "dokploy_environment" "test_redis" {
  project_id = dokploy_project.test_redis.id
  name       = "%s"
}

resource "dokploy_redis" "test" {
  name              = "%s"
  app_name_prefix   = "%s"
  database_password = "test_redis_password_123"
  environment_id    = dokploy_environment.test_redis.id
}

resource "dokploy_destination" "test_redis" {
  name              = "%s"
  storage_provider  = "s3"
  access_key        = "test-access-key"
  secret_access_key = "test-secret-key"
  bucket            = "test-redis-vol-backups"
  region            = "us-east-1"
  endpoint          = "https://s3.amazonaws.com"
}

resource "dokploy_volume_backup" "test_redis" {
  name            = "redis-vol-backup"
  volume_name     = "redis_data"
  prefix          = "redis-vol"
  destination_id  = dokploy_destination.test_redis.id
  cron_expression = "0 2 * * *"
  service_type    = "redis"
  service_id      = dokploy_redis.test.id
  app_name        = dokploy_redis.test.app_name
  keep_latest_count = 3
  enabled         = true
}
`, os.Getenv("DOKPLOY_HOST"), os.Getenv("DOKPLOY_API_KEY"), projectName, envName, redisName, appNamePrefix, destName)
}

func testAccVolumeBackupsDataSourceConfig(projectName, envName, pgName, appName, dbName, dbUser, destName string) string {
	return fmt.Sprintf(`
provider "dokploy" {
  host    = "%s"
  api_key = "%s"
}

resource "dokploy_project" "test_ds" {
  name        = "%s"
  description = "Test project for volume backups data source"
}

resource "dokploy_environment" "test_ds" {
  project_id = dokploy_project.test_ds.id
  name       = "%s"
}

resource "dokploy_postgres" "test_ds" {
  name              = "%s"
  app_name_prefix   = "%s"
  database_name     = "%s"
  database_user     = "%s"
  database_password = "test_password_123"
  environment_id    = dokploy_environment.test_ds.id
}

resource "dokploy_destination" "test_ds" {
  name              = "%s"
  storage_provider  = "s3"
  access_key        = "test-access-key"
  secret_access_key = "test-secret-key"
  bucket            = "test-ds-vol-backups"
  region            = "us-east-1"
  endpoint          = "https://s3.amazonaws.com"
}

resource "dokploy_volume_backup" "test_ds" {
  name            = "ds-vol-backup"
  volume_name     = "postgres_data"
  prefix          = "ds-vol"
  destination_id  = dokploy_destination.test_ds.id
  cron_expression = "0 5 * * *"
  service_type    = "postgres"
  service_id      = dokploy_postgres.test_ds.id
  app_name        = dokploy_postgres.test_ds.app_name
}

data "dokploy_volume_backups" "test" {
  service_id   = dokploy_postgres.test_ds.id
  service_type = "postgres"

  depends_on = [dokploy_volume_backup.test_ds]
}
`, os.Getenv("DOKPLOY_HOST"), os.Getenv("DOKPLOY_API_KEY"), projectName, envName, pgName, appName, dbName, dbUser, destName)
}
