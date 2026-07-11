<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { Code24Regular, Warning24Regular, Edit24Regular } from '@vicons/fluent'
import { loadATTemplates, saveATTemplates, resetATTemplates, type ATTemplateGroup } from '../constants/atTemplates'
import { devicesService } from '../services/devices'

const props = defineProps<{
  deviceId: string
  backendMode?: string
  atPort?: string
  running?: boolean
}>()

const atCmd = ref('')
const atTemplate = ref('')
const atTimeoutMs = ref(10000)
const atSending = ref(false)
const atHistory = ref<Array<{ ts: number; cmd: string; ok: boolean; response: string }>>([])

// 模板选择后自动填充到命令框
watch(() => atTemplate.value, (v) => {
  const cmd = String(v || '').trim()
  if (cmd) atCmd.value = cmd
})

// AT 暂停/恢复状态
const atPaused = ref(false)
const atPausePending = ref(false)

// 模板编辑
const showTemplateEditor = ref(false)
const templateEditorText = ref('')
const templateEditorError = ref('')

const atTemplates = ref<ATTemplateGroup[]>([])

function refreshTemplates() {
  atTemplates.value = loadATTemplates()
}

const hasATPort = computed(() => String(props.atPort || '').trim().length > 0)
const canUseATTerminal = computed(() => Boolean(props.running) && hasATPort.value)
const needsPauseButton = computed(() => {
  const mode = String(props.backendMode || '').toLowerCase()
  // QMI 模式下后台 AT 检测不运行，不存在串口冲突，无需暂停
  return mode === 'mbim'
})

// 挂载时同步状态
onMounted(async () => {
  refreshTemplates()
  if (props.deviceId && props.running && hasATPort.value && needsPauseButton.value) {
    try {
      const result = await devicesService.getATPauseStatus(props.deviceId)
      if (result.ok) atPaused.value = result.data
    } catch { /* 忽略 */ }
  }
})

async function doPause(): Promise<boolean> {
  if (!props.deviceId) return false
  atPausePending.value = true
  try {
    const result = await devicesService.pauseAT(props.deviceId)
    if (result.ok) { atPaused.value = true; return true }
    return false
  } finally {
    atPausePending.value = false
  }
}

async function doResume(): Promise<boolean> {
  if (!props.deviceId) return false
  atPausePending.value = true
  try {
    const result = await devicesService.resumeAT(props.deviceId)
    if (result.ok) { atPaused.value = false; return true }
    return false
  } finally {
    atPausePending.value = false
  }
}

async function sendAT() {
  const cmd = String(atCmd.value || '').trim()
  if (!cmd) return

  if (needsPauseButton.value && !atPaused.value) {
    try {
      await ElMessageBox.confirm(
        `<div class="at-confirm-message">
          <div class="at-confirm-title">
            <span class="at-confirm-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
            </span>
            暂停后台检测
          </div>
          <div class="at-confirm-desc">
            后台 AT 检测正在运行，直接发送命令可能导致串口冲突。点击「暂停并发送」后将自动暂停并发送。
          </div>
        </div>`,
        '',
        {
          confirmButtonText: '暂停并发送',
          cancelButtonText: '取消',
          showClose: false,
          closeOnClickModal: false,
          closeOnPressEscape: false,
          customClass: 'at-pause-confirm',
          dangerouslyUseHTMLString: true,
        }
      )
    } catch { return }
    const ok = await doPause()
    if (!ok) {
      atHistory.value.push({ ts: Date.now(), cmd, ok: false, response: '暂停后台检测失败，请重试' })
      return
    }
  }

  atSending.value = true
  atCmd.value = ''
  try {
    const result = await devicesService.sendAT(props.deviceId, { cmd, timeout_ms: atTimeoutMs.value || 10000 })
    if (!result.ok) throw new Error(result.error.message || '请求异常')
    atHistory.value.push({ ts: Date.now(), cmd, ok: result.data.ok, response: result.data.response })
  } catch (e: unknown) {
    atHistory.value.push({ ts: Date.now(), cmd, ok: false, response: e instanceof Error ? e.message : '请求异常' })
  } finally {
    atSending.value = false
  }
}

async function toggleATPause(newVal: string | number | boolean) {
  if (newVal) { await doPause() } else { await doResume() }
}

function clearATHistory() { atHistory.value = [] }

// ===== 模板编辑器 =====
function openTemplateEditor() {
  templateEditorText.value = JSON.stringify(atTemplates.value, null, 2)
  templateEditorError.value = ''
  showTemplateEditor.value = true
}
function applyTemplateEdit() {
  try {
    const parsed = JSON.parse(templateEditorText.value)
    if (!Array.isArray(parsed)) throw new Error('模板必须是数组格式')
    for (const g of parsed) {
      if (!g.label || typeof g.label !== 'string') throw new Error('每个分组必须有 label 字段')
      if (!Array.isArray(g.items)) throw new Error(`分组 "${g.label}" 的 items 必须是数组`)
      for (const it of g.items) {
        if (!it.label || typeof it.label !== 'string') throw new Error('每条模板必须有 label')
        if (!it.value || typeof it.value !== 'string') throw new Error('每条模板必须有 value')
      }
    }
    saveATTemplates(parsed as ATTemplateGroup[])
    refreshTemplates()
    showTemplateEditor.value = false
    templateEditorError.value = ''
  } catch (e: unknown) {
    templateEditorError.value = e instanceof Error ? e.message : 'JSON 格式错误'
  }
}
function resetToDefault() {
  atTemplates.value = resetATTemplates()
  templateEditorText.value = JSON.stringify(atTemplates.value, null, 2)
  templateEditorError.value = ''
}
</script>

<template>
  <div>
    <!-- 标题行 -->
    <div class="flex items-center gap-3">
      <div class="w-10 h-10 rounded-xl bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-gray-700 dark:text-gray-300">
        <el-icon size="22"><Code24Regular /></el-icon>
      </div>
      <div class="flex-1">
        <div class="text-lg font-bold text-gray-900 dark:text-white">AT 终端</div>
        <div class="text-sm text-gray-500 dark:text-gray-400 mt-0.5">发送 AT 指令并查看回显</div>
      </div>
      <div v-if="canUseATTerminal && needsPauseButton" class="flex items-center gap-2">
        <span class="text-xs text-gray-400">后台检测</span>
        <el-switch
          :model-value="atPaused"
          :loading="atPausePending"
          active-text="暂停"
          inactive-text="运行"
          inline-prompt
          style="--el-switch-on-color: #f59e0b; --el-switch-off-color: #10b981"
          @change="toggleATPause"
        />
      </div>
    </div>

    <!-- 暂停提示条 -->
    <div v-if="atPaused && canUseATTerminal" class="mt-3 px-3 py-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800/50 text-xs text-amber-700 dark:text-amber-300 flex items-center gap-2">
      <el-icon size="16"><Warning24Regular /></el-icon>
      <span>后台检测已暂停，端口独占中。离开 AT 终端时请注意恢复。</span>
    </div>

    <!-- 不可用状态 -->
    <template v-if="!canUseATTerminal">
      <div class="mt-4 p-8 flex flex-col items-center justify-center bg-orange-50 dark:bg-orange-900/20 border border-orange-100 dark:border-orange-900/50 rounded-xl">
        <el-icon size="48" class="text-orange-400 mb-4"><Warning24Regular /></el-icon>
        <div class="text-lg font-bold text-orange-700 dark:text-orange-400">AT 终端暂不可用</div>
        <div class="text-sm text-orange-600 dark:text-orange-300 mt-2 text-center max-w-md">
          {{ !props.running ? '设备未运行' : '当前设备没有可用 AT 端口' }}
        </div>
      </div>
    </template>

    <template v-else>
      <!-- 交互历史 -->
      <div class="ui-panel-muted mt-4 p-4 h-[320px] overflow-auto flex flex-col gap-3 rounded-xl border border-gray-100 dark:border-white/10 relative">
        <div v-if="atHistory.length === 0 && !atSending" class="absolute inset-0 flex items-center justify-center text-sm text-gray-400">暂无 AT 会话记录</div>
        <div v-for="(h, i) in atHistory" :key="h.ts + h.cmd + i" class="flex flex-col gap-2 w-full">
          <div class="flex w-full justify-end">
            <div class="max-w-[80%] bg-indigo-500 text-white rounded-2xl rounded-tr-sm px-4 py-2.5 shadow-sm">
              <div class="text-sm font-mono break-words">{{ h.cmd }}</div>
              <div class="text-[10px] text-indigo-100 mt-1 text-right">{{ new Date(h.ts).toLocaleTimeString() }}</div>
            </div>
          </div>
          <div class="flex w-full justify-start">
            <div class="max-w-[80%] rounded-2xl rounded-tl-sm px-4 py-2.5 shadow-sm" :class="!h.ok ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-300 border border-red-100 dark:border-red-900/50' : 'bg-white dark:bg-gray-800 text-gray-800 dark:text-gray-200 border border-gray-100 dark:border-white/5'">
              <div class="text-sm whitespace-pre-wrap break-words font-mono">{{ h.response }}</div>
              <div class="text-[10px] mt-1 text-gray-400">{{ new Date(h.ts).toLocaleTimeString() }}</div>
            </div>
          </div>
        </div>
        <div v-if="atSending" class="flex w-full justify-start mt-2">
          <div class="max-w-[80%] bg-white dark:bg-gray-800 rounded-2xl rounded-tl-sm px-4 py-3 shadow-sm border border-gray-100 dark:border-white/5 flex items-center gap-2">
            <div class="flex space-x-1">
              <div class="w-1.5 h-1.5 bg-indigo-400 rounded-full animate-bounce [animation-delay:-0.3s]"></div>
              <div class="w-1.5 h-1.5 bg-indigo-400 rounded-full animate-bounce [animation-delay:-0.15s]"></div>
              <div class="w-1.5 h-1.5 bg-indigo-400 rounded-full animate-bounce"></div>
            </div>
            <span class="text-xs text-gray-400 ml-1">等待模组响应…</span>
          </div>
        </div>
      </div>

      <!-- 输入区 -->
      <div class="grid grid-cols-1 md:grid-cols-[200px_1fr_110px_auto] gap-3 mt-4">
        <div class="space-y-1">
          <div class="text-[11px] font-bold text-gray-500 uppercase tracking-wider flex items-center justify-between">
            <span>模板</span>
            <el-button size="small" text type="primary" @click="openTemplateEditor" class="!p-0 !h-auto">
              <el-icon size="14"><Edit24Regular /></el-icon>
            </el-button>
          </div>
          <el-select v-model="atTemplate" filterable clearable placeholder="选择命令">
            <el-option-group v-for="g in atTemplates" :key="g.label" :label="g.label">
              <el-option v-for="it in g.items" :key="it.value" :label="it.label" :value="it.value" />
            </el-option-group>
          </el-select>
        </div>
        <div class="space-y-1">
          <div class="text-[11px] font-bold text-gray-500 uppercase tracking-wider">命令</div>
          <el-input v-model="atCmd" placeholder="AT+CSQ" @keyup.enter="sendAT" :disabled="atSending" />
        </div>
        <div class="space-y-1">
          <div class="text-[11px] font-bold text-gray-500 uppercase tracking-wider">超时 ms</div>
          <el-input v-model.number="atTimeoutMs" type="number" placeholder="10000" />
        </div>
        <div class="space-y-1 self-end">
          <div class="text-[11px] font-bold text-gray-500 uppercase tracking-wider opacity-0 select-none">操作</div>
          <div class="flex items-center justify-end gap-2">
            <el-button type="default" @click="clearATHistory" class="ui-button-plain">清空</el-button>
            <el-button type="primary" :loading="atSending" :disabled="!atCmd" @click="sendAT" class="!border-0">发送</el-button>
          </div>
        </div>
      </div>
    </template>

    <!-- 模板编辑器 -->
    <el-dialog v-model="showTemplateEditor" title="编辑 AT 模板" width="700px" :close-on-click-modal="false">
      <div class="text-xs text-gray-500 mb-2">JSON 格式，保存后立即生效（存储在浏览器本地）。</div>
      <el-input v-model="templateEditorText" type="textarea" :rows="20" placeholder="JSON 模板数组" class="font-mono text-sm" />
      <div v-if="templateEditorError" class="mt-2 text-sm text-red-500">{{ templateEditorError }}</div>
      <template #footer>
        <div class="flex justify-between">
          <el-button type="warning" text @click="resetToDefault">恢复默认</el-button>
          <div class="flex gap-2">
            <el-button @click="showTemplateEditor = false">取消</el-button>
            <el-button type="primary" @click="applyTemplateEdit">保存</el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style>
/* 美化 AT 暂停确认弹窗（Element Plus MessageBox 挂载在 body 下，需要全局样式） */
.at-pause-confirm {
  --at-confirm-primary: #f59e0b;
  --at-confirm-bg: #ffffff;
  --at-confirm-text: #1f2937;
  --at-confirm-muted: #64748b;
  border-radius: 16px !important;
  box-shadow: 0 20px 60px -12px rgba(0, 0, 0, 0.18) !important;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Helvetica Neue", Helvetica, Arial, sans-serif;
  max-width: 380px;
  width: 90%;
  padding: 0;
  overflow: hidden;
}

html.dark .at-pause-confirm {
  --at-confirm-bg: #1f2937;
  --at-confirm-text: #f3f4f6;
  --at-confirm-muted: #9ca3af;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.at-pause-confirm .el-message-box__header {
  display: none;
}

.at-pause-confirm .el-message-box__content {
  padding: 28px 24px 16px !important;
  background: var(--at-confirm-bg);
  color: var(--at-confirm-text);
}

.at-confirm-message {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.at-confirm-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 16px;
  font-weight: 600;
  color: var(--at-confirm-text);
  line-height: 1.3;
}

.at-confirm-icon {
  width: 32px;
  height: 32px;
  border-radius: 10px;
  background: rgba(245, 158, 11, 0.12);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--at-confirm-primary);
  flex-shrink: 0;
}

.at-confirm-icon svg {
  width: 18px;
  height: 18px;
}

.at-confirm-desc {
  font-size: 13px;
  color: var(--at-confirm-muted);
  line-height: 1.7;
  padding-left: 42px;
}

.at-pause-confirm .el-message-box__btns {
  padding: 10px 24px 24px !important;
  background: var(--at-confirm-bg);
  border-top: none;
}

.at-pause-confirm .el-message-box__btns .el-button {
  border-radius: 10px;
  font-size: 13px;
  font-weight: 500;
  padding: 8px 18px;
  transition: transform 0.12s ease, box-shadow 0.12s ease;
}

.at-pause-confirm .el-message-box__btns .el-button--primary {
  background: linear-gradient(135deg, #f59e0b, #f97316);
  border: none;
  box-shadow: 0 4px 14px rgba(245, 158, 11, 0.35);
}

.at-pause-confirm .el-message-box__btns .el-button--primary:hover {
  background: linear-gradient(135deg, #f97316, #ea580c);
  transform: translateY(-1px);
  box-shadow: 0 6px 18px rgba(245, 158, 11, 0.42);
}

.at-pause-confirm .el-message-box__btns .el-button:not(.el-button--primary) {
  color: var(--at-confirm-muted);
  background: rgba(156, 163, 175, 0.1);
  border: none;
}

.at-pause-confirm .el-message-box__btns .el-button:not(.el-button--primary):hover {
  background: rgba(156, 163, 175, 0.18);
  color: var(--at-confirm-text);
}
</style>
