datacenter           = "dc1"
node_name            = "persister-consul"
server               = true
bootstrap_expect     = 1
data_dir             = "/consul/data"
client_addr          = "0.0.0.0"
bind_addr            = "0.0.0.0"
disable_update_check = true

ui_config {
  enabled = true
}