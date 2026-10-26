package configgen

// configTemplate 是融合模式 / 自定义模式的基础骨架。
//
// 其中不写 ipv6：它属于生成期设置（config.IPv6），由 applyFluxorFields 统一写入，
// 避免「模板与设置两处真相」。tun 块同理，见 TunBlock——它是静态安全基线，
// 不承载用户偏好。
const configTemplate = `mixed-port:
tproxy-port:
allow-lan: true
bind-address: '*'
mode: rule
log-level: silent
unified-delay: true
external-controller: ''
external-controller-unix: '/var/apps/Fluxor/target/core.sock'
external-ui:
external-ui-url:
secret: ''

routing-mark: 255
find-process-mode: strict
client-fingerprint: chrome

profile:
  store-selected: true
  store-fake-ip: true

sniffer:
  enable: true
  sniff:
    HTTP:
      ports: [80, 8080-8880]
      override-destination: true
    TLS:
      ports: [443, 8443]
    QUIC:
      ports: [443, 8443]
  skip-domain:
    - "+.push.apple.com"
` + TunBlock + `
geodata-mode: false
geo-auto-update: true
geo-update-interval: 24
`
