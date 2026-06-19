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
<<<<<<< HEAD
        success: 'var(--success)',
        danger: 'var(--danger)',
        warning: 'var(--warning)',
=======
>>>>>>> 43c7c27f16564dee02a428f34317c113f471df69
        brand: {
          50: '#eff6ff',
          100: '#dbeafe',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
        }
      }
    },
  },
  plugins: [],
}
