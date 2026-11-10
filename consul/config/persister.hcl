service {
  name    = "persister"
  address = "persister"
  port    = 8083
  tags    = ["documents","internal","v1"]
  meta    = { role = "persister" }
  check {
    id        = "persister-health"
    name      = "persister /api/v1/health"
    http      = "http://persister:8083/api/v1/health"
    interval  = "10s"
    timeout   = "3s"
  }
}
