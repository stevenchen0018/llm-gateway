import { ElMessage } from 'element-plus'

// The async Clipboard API only exists in secure contexts (HTTPS or localhost).
// The console is often opened over plain http on an intranet IP or hostname,
// where navigator.clipboard is undefined — so fall back to a hidden textarea
// and document.execCommand('copy'), which works on any origin during a click.
function legacyCopy(text: string): boolean {
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.cssText = 'position:fixed;top:0;left:0;width:1px;height:1px;padding:0;border:0;opacity:0;'
  // inside an open dialog/drawer the focus trap would steal focus from a
  // textarea appended to <body>, so put it next to the clicked element
  const host = (document.activeElement?.closest('.el-dialog, .el-drawer, .el-overlay') as HTMLElement | null) ?? document.body
  const prev = document.activeElement as HTMLElement | null
  host.appendChild(ta)
  ta.focus()
  ta.select()
  ta.setSelectionRange(0, text.length)
  let ok = false
  try { ok = document.execCommand('copy') } catch { ok = false }
  host.removeChild(ta)
  prev?.focus?.()
  return ok
}

/** copyText copies text and reports the outcome; returns whether it worked. */
export async function copyText(text: string, okMsg = '已复制'): Promise<boolean> {
  let ok = false
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); ok = true } catch { ok = legacyCopy(text) }
  } else {
    ok = legacyCopy(text)
  }
  if (ok) ElMessage.success(okMsg)
  else ElMessage.warning('浏览器不允许自动复制，请手动选中内容后按 Ctrl/⌘ + C')
  return ok
}
