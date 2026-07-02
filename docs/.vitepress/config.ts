import { defineConfig } from 'vitepress'

export default defineConfig({
  title: "Fluxor",
  description: "Mihomo 内核轻量级管理面板与订阅生成系统使用手册",
  base: "/Fluxor/",
  themeConfig: {
    nav: [
      { text: '首页', link: '/' },
      { text: '快速开始', link: '/quick-start' },
      { text: '使用手册', link: '/usage-guide' }
    ],
    sidebar: [
      {
        text: '文档指南',
        items: [
          { text: '快速开始', link: '/quick-start' },
          { text: '配置指南', link: '/config-guide' },
          { text: '使用指南', link: '/usage-guide' },
          { text: '常见问题解答', link: '/faq' }
        ]
      }
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/shuangji66/fluxor' }
    ]
  }
})
