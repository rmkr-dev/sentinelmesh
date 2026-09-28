variable "name" { type = string }
variable "location" { type = string }
variable "resource_group_name" { type = string }
variable "alert_email" { type = string }
variable "tags" { type = map(string) }

resource "azurerm_log_analytics_workspace" "this" {
  name                = "${var.name}-law"
  location            = var.location
  resource_group_name = var.resource_group_name
  sku                 = "PerGB2018"
  retention_in_days   = 30
  tags                = var.tags
}

resource "azurerm_application_insights" "this" {
  name                = "${var.name}-appi"
  location            = var.location
  resource_group_name = var.resource_group_name
  workspace_id        = azurerm_log_analytics_workspace.this.id
  application_type    = "other"
  tags                = var.tags
}

resource "azurerm_monitor_workspace" "this" {
  name                = "${var.name}-amw"
  location            = var.location
  resource_group_name = var.resource_group_name
  tags                = var.tags
}

resource "azurerm_monitor_data_collection_endpoint" "otlp" {
  name                = "${var.name}-dce"
  location            = var.location
  resource_group_name = var.resource_group_name
  kind                = "Linux"
  tags                = var.tags
}

resource "azurerm_monitor_action_group" "sre" {
  name                = "${var.name}-sre"
  resource_group_name = var.resource_group_name
  short_name          = "sre"
  tags                = var.tags

  email_receiver {
    name                    = "sre"
    email_address           = var.alert_email
    use_common_alert_schema = true
  }
}

resource "azurerm_monitor_metric_alert" "appi_failures" {
  name                = "${var.name}-failed-requests"
  resource_group_name = var.resource_group_name
  scopes              = [azurerm_application_insights.this.id]
  description         = "Application Insights failed requests. This is a signal, not a root cause."
  severity            = 2
  frequency           = "PT1M"
  window_size         = "PT5M"
  tags                = var.tags

  criteria {
    metric_namespace = "microsoft.insights/components"
    metric_name      = "requests/failed"
    aggregation      = "Count"
    operator         = "GreaterThan"
    threshold        = 20
  }

  action {
    action_group_id = azurerm_monitor_action_group.sre.id
  }
}

output "workspace_id" { value = azurerm_log_analytics_workspace.this.id }
output "monitor_workspace_id" { value = azurerm_monitor_workspace.this.id }
output "connection_string" {
  value     = azurerm_application_insights.this.connection_string
  sensitive = true
}
output "dce_id" { value = azurerm_monitor_data_collection_endpoint.otlp.id }
output "action_group_id" { value = azurerm_monitor_action_group.sre.id }
