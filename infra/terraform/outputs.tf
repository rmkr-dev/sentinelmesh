output "resource_group" {
  value = module.resource_group.name
}

output "log_analytics_workspace_id" {
  value = module.monitoring.workspace_id
}

output "application_insights_connection_string" {
  value     = module.monitoring.connection_string
  sensitive = true
}

output "azure_monitor_workspace_id" {
  value = module.monitoring.monitor_workspace_id
}

output "collector_identity_client_id" {
  value = module.identity.client_id
}

output "acr_login_server" {
  value = module.acr.login_server
}

output "aks_name" {
  value = try(module.aks[0].name, null)
}

output "grafana_endpoint" {
  value = try(module.grafana[0].endpoint, null)
}

output "profile" {
  value = var.profile
}
