/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  darkMode: ['class', '[data-theme="dark"]'], // 兼容 data-theme 属性切换
  theme: {
    extend: {
      colors: {
        accent: 'var(--accent)',
        'accent-hover': 'var(--accent-hover)',
        success: 'var(--success)',
        'success-hover': 'var(--success-hover)',
        danger: 'var(--danger)',
        'danger-hover': 'var(--danger-hover)',
        warning: 'var(--warning)',
        'warning-hover': 'var(--warning-hover)',
        // Apple-design-analysis 规范主题变量映射
        'apple-bg': 'var(--bg-primary)',
        'apple-card': 'var(--bg-secondary)',
        'apple-input': 'var(--bg-input)',
        'apple-text': 'var(--text-primary)',
        'apple-text-muted': 'var(--text-secondary)',
        'apple-border': 'var(--border-color)',
        brand: {
          50: '#eff6ff',
          100: '#dbeafe',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
        }
      },
      borderRadius: {
        xs: '5px',
        sm: '8px',
        md: '11px',
        lg: '18px',
      }
    },
  },
  plugins: [],
}

