import { defineConfig } from 'vitepress'

export default defineConfig({
  title: "Fluxor",
  description: "Mihomo 内核轻量级管理面板与订阅生成系统使用手册",
  base: "/fluxor/",
  themeConfig: {
    nav: [
      { text: '首页', link: '/' },
      { text: '使用手册', link: '/user-manual' },
      { text: '常见问题', link: '/faq' }
    ],
    sidebar: [
      {
        text: '使用指南',
        items: [
          { text: '用户使用手册', link: '/user-manual' },
          { text: '常见问题解答', link: '/faq' }
        ]
      }
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/shuangji66/fluxor' }
    ]
  }
})
