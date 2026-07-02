import { defineConfig } from 'vitepress'

export default defineConfig({
  title: "Fluxor",
  description: "Mihomo 内核轻量级管理面板与订阅生成系统使用手册",
  // @ts-ignore
  base: process.env.CF_PAGES ? '/' : '/fluxor/',
  head: [
    ['link', { rel: 'icon', href: '/logo.png' }]
  ],
  themeConfig: {
    logo: '/logo.png',
    nav: [
      { text: '首页', link: '/' },
      { text: '指南', link: '/guide/introduction' },
      { text: '配置', link: '/config/file-structure' },
      { text: '使用', link: '/usage/views' },
      { text: '常见问题', link: '/faq' }
    ],
    search: {
      provider: 'local'
    },
    sidebar: {
      '/guide/': [
        {
          text: '关于项目',
          collapsed: false,
          items: [
            { text: '什么是 Fluxor', link: '/guide/introduction' },
            { text: '功能特性', link: '/guide/features' }
          ]
        },
        {
          text: '新手入门',
          collapsed: false,
          items: [
            { text: '快速开始', link: '/guide/quick-start' }
          ]
        }
      ],
      '/config/': [
        {
          text: '系统配置参考',
          collapsed: false,
          items: [
            { text: '目录与路径', link: '/config/file-structure' },
            { text: '持久化配置 (fluxor.json)', link: '/config/fluxor-json' }
          ]
        },
        {
          text: '高级进阶原理',
          collapsed: false,
          items: [
            { text: '订阅工作模式', link: '/config/subscription-modes' },
            { text: 'TProxy 透明代理', link: '/config/tproxy' }
          ]
        }
      ],
      '/usage/': [
        {
          text: '日常使用操作',
          collapsed: false,
          items: [
            { text: '面板界面与操作', link: '/usage/views' }
          ]
        }
      ]
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/shuangji66/fluxor' }
    ]
  }
})