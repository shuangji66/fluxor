import { ref, onUnmounted, type Ref } from 'vue'

/**
 * 响应式媒体查询：返回随视口变化的布尔 ref。
 *
 * 为什么不直接比较 `window.innerWidth`：
 * 内嵌 WebView / 应用内浏览器里，窗口宽度可能在整个生命周期内都与真实布局视口
 * 不一致（页面已按 device-width 排版，`innerWidth` 仍停在排版前的宽度），而 CSS
 * 断点由渲染引擎按真实布局视口求值。两边一旦分歧，同一页面就会出现自相矛盾的
 * 布局——例如底部是移动端导航栏、代理组却排成双列：双列时卡片只有 ~160px 宽，
 * 组名被挤到 0 宽、只剩类型徽标，当前节点行截断成「当…」。
 *
 * matchMedia 与 Tailwind 的断点走同一个求值器，不可能与 CSS 分歧；监听 change
 * 还能覆盖缩放、分屏、旋转等不触发 resize 的变更。
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const mql = window.matchMedia(query)
  const matches = ref(mql.matches)
  const onChange = (e: MediaQueryListEvent) => { matches.value = e.matches }
  mql.addEventListener('change', onChange)
  onUnmounted(() => mql.removeEventListener('change', onChange))
  return matches
}
