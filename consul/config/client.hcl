datacenter = "dc-converter"
node_name  = "persister-consul-client"
server     = false
retry_join = ["consul"]
data_dir   = "/consul/data"
client_addr = "0.0.0.0"
bind_addr   = "0.0.0.0"
disable_update_check = true
