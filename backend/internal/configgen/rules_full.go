package configgen

// rulesFull 是 full（详细）规则集的规则模板。
//
// 开头 6 条 DOMAIN 规则服务于 Fluxor 自己：概览页的「代理出口 IP / 归属地」由
// netinfo 经本机 HTTP 代理请求下列地址（见 backend/internal/netinfo/handlers.go），
// 而这些域名会命中 networktest 规则集，该组默认选中「🎯 全球直连」——于是查询打到
// 了本机出口，卡片上显示的其实是本机归属地而不是节点归属地。
//
// 因此把「代理出口 IP 查询」用的域名显式指向「🚀 节点选择」，并放在全部 RULE-SET
// 之前（rules 首个匹配生效，规则集更新也改不动这一行为）。地址清单必须与
// netinfo/handlers.go 中 HandleProxyIPv4 / HandleProxyIPv6 的 urls 保持一致：
// 新增查询源时两处同步，否则新源仍会被规则集分流走。
const rulesFull = `
rules:
  - DOMAIN,api.ipify.org,🚀 节点选择
  - DOMAIN,api6.ipify.org,🚀 节点选择
  - DOMAIN,ipv4.icanhazip.com,🚀 节点选择
  - DOMAIN,ipv6.icanhazip.com,🚀 节点选择
  - DOMAIN,v4.ident.me,🚀 节点选择
  - DOMAIN,v6.ident.me,🚀 节点选择
  - RULE-SET,private,🔒 私有网络
  - RULE-SET,ads,🛑 广告域名
  - RULE-SET,trackerslist,📋 Trackerslist
  - RULE-SET,applications,⬇️ 直连软件
  - RULE-SET,microsoft-cn,🪟 微软服务
  - RULE-SET,apple-cn,🍎 苹果服务
  - RULE-SET,google-cn,🇬 谷歌服务
  - RULE-SET,games-cn,🎮 游戏服务
  - RULE-SET,games,🕹️ 游戏平台
  - RULE-SET,netflix,🎥 奈飞视频
  - RULE-SET,disney,📽️ 迪士尼+
  - RULE-SET,max,🎞️ Max
  - RULE-SET,primevideo,🎬 Prime Video
  - RULE-SET,appletv,🍎 Apple TV+
  - RULE-SET,youtube,📹 油管视频
  - RULE-SET,tiktok,🎵 TikTok
  - RULE-SET,bilibili,📺 哔哩哔哩
  - RULE-SET,spotify,🎶 Spotify
  - RULE-SET,media,🌍 国外媒体
  - RULE-SET,ai,🤖 AI 平台
  - RULE-SET,networktest,📈 网络测试
  - RULE-SET,tld-proxy,🌎 国外顶级域名
  - RULE-SET,gfw,🌎 国外域名
  - RULE-SET,proxy,🌎 国外域名
  - RULE-SET,cn,🇨🇳 国内域名
  - RULE-SET,privateip,🔒 私有网络,no-resolve
  - RULE-SET,cnip,🀄️ 国内 IP
  - RULE-SET,telegramip,📲 电报消息,no-resolve
  - RULE-SET,netflixip,🎥 奈飞视频
  - RULE-SET,mediaip,🌍 国外媒体
  - MATCH,🐟 漏网之鱼
`
