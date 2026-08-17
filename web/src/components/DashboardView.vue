<script setup lang="ts">
import type { Backup, Diagnostics, Status, UpdateRecord } from '../types'

const props = defineProps<{
  status: Status | null
  history: UpdateRecord[]
  backups: Backup[]
  diagnostics: Diagnostics | null
  loading: boolean
  action: 'update' | 'restore' | null
  error: string
}>()

const emit = defineEmits<{
  refresh: []
  update: []
  restore: [id: string]
  logout: []
}>()

function formatDate(value?: string): string {
  if (!value) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(value))
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

function confirmRestore(backup: Backup): void {
  if (props.action) return
  const confirmed = window.confirm(`确定恢复备份 ${formatDate(backup.created_at)} 吗？当前配置会先自动备份。`)
  if (confirmed) emit('restore', backup.id)
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <div class="brand">
        <span class="brand-mark small" aria-hidden="true">M</span>
        <div>
          <strong>Mihomo Stack</strong>
          <span>安全配置控制台</span>
        </div>
      </div>
      <nav aria-label="控制台操作">
        <a v-if="status?.dashboard_url" class="button ghost" :href="status.dashboard_url" target="_blank" rel="noreferrer">
          打开代理面板
        </a>
        <button class="ghost" type="button" :disabled="loading" @click="emit('refresh')">刷新</button>
        <button class="ghost" type="button" @click="emit('logout')">退出</button>
      </nav>
    </header>

    <main class="content">
      <div class="hero-row">
        <div>
          <p class="eyebrow">OVERVIEW</p>
          <h1>配置运行概览</h1>
          <p class="muted">订阅更新只有在 Mihomo 验证和健康检查通过后才会生效。</p>
        </div>
        <button
          class="primary"
          type="button"
          :disabled="Boolean(action) || status?.update_busy"
          @click="emit('update')"
        >
          {{ action === 'update' ? '正在安全更新…' : '立即更新配置' }}
        </button>
      </div>

      <p v-if="error" class="alert error" role="alert">{{ error }}</p>

      <section class="metric-grid" aria-label="服务状态">
        <article class="metric-card">
          <span class="metric-label">Mihomo</span>
          <strong><i :class="['status-dot', status?.mihomo.online ? 'online' : 'offline']" />{{ status?.mihomo.online ? '运行正常' : '连接失败' }}</strong>
          <small>{{ status?.mihomo.version || '版本未知' }}</small>
        </article>
        <article class="metric-card">
          <span class="metric-label">订阅源</span>
          <strong>{{ status?.source_count ?? '—' }} 个</strong>
          <small>由秘密文件安全加载</small>
        </article>
        <article class="metric-card">
          <span class="metric-label">定时更新</span>
          <strong>{{ status?.scheduler_enabled ? '已开启' : '已关闭' }}</strong>
          <small>{{ status?.scheduler_enabled ? `每 ${status.update_interval}` : '仅允许手动更新' }}</small>
        </article>
        <article class="metric-card">
          <span class="metric-label">最近结果</span>
          <strong>{{ status?.last_update ? (status.last_update.success ? '更新成功' : '更新失败') : '暂无记录' }}</strong>
          <small>{{ formatDate(status?.last_update?.finished_at) }}</small>
        </article>
      </section>

      <div class="two-column">
        <section class="panel">
          <div class="panel-heading">
            <div>
              <p class="eyebrow">HISTORY</p>
              <h2>更新记录</h2>
            </div>
            <span>{{ history.length }} 条</span>
          </div>
          <div v-if="history.length" class="table-wrap">
            <table>
              <thead><tr><th>结果</th><th>触发方式</th><th>阶段</th><th>完成时间</th></tr></thead>
              <tbody>
                <tr v-for="item in history" :key="item.id">
                  <td><span :class="['pill', item.success ? 'success' : 'danger']">{{ item.success ? '成功' : (item.rolled_back ? '已回滚' : '失败') }}</span></td>
                  <td>{{ item.trigger }}</td>
                  <td>{{ item.stage }}</td>
                  <td>{{ formatDate(item.finished_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="empty">还没有更新记录。</p>
        </section>

        <section class="panel">
          <div class="panel-heading">
            <div>
              <p class="eyebrow">BACKUPS</p>
              <h2>配置备份</h2>
            </div>
            <span>{{ backups.length }} 份</span>
          </div>
          <ul v-if="backups.length" class="backup-list">
            <li v-for="backup in backups" :key="backup.id">
              <div>
                <strong>{{ formatDate(backup.created_at) }}</strong>
                <small>{{ formatBytes(backup.size) }}</small>
              </div>
              <button class="small-button" type="button" :disabled="Boolean(action)" @click="confirmRestore(backup)">
                {{ action === 'restore' ? '处理中…' : '恢复' }}
              </button>
            </li>
          </ul>
          <p v-else class="empty">首次替换配置后会自动生成备份。</p>
        </section>
      </div>

      <section class="panel diagnostics">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">DIAGNOSTICS</p>
            <h2>运行诊断</h2>
          </div>
        </div>
        <dl v-if="diagnostics">
          <div><dt>Guardian</dt><dd>{{ diagnostics.guardian.version }}</dd></div>
          <div><dt>运行平台</dt><dd>{{ diagnostics.guardian.os }}/{{ diagnostics.guardian.arch }}</dd></div>
          <div><dt>Go Runtime</dt><dd>{{ diagnostics.guardian.go }}</dd></div>
          <div><dt>当前配置</dt><dd>{{ diagnostics.active_config.exists ? `${formatBytes(diagnostics.active_config.size ?? 0)} · ${formatDate(diagnostics.active_config.modified_at)}` : '尚未生成' }}</dd></div>
          <div><dt>身份认证</dt><dd>{{ diagnostics.authentication_enabled ? '已启用' : '仅限本机免认证' }}</dd></div>
          <div><dt>Controller</dt><dd>{{ diagnostics.mihomo_reachable ? '可访问' : '不可访问' }}</dd></div>
        </dl>
      </section>
    </main>
  </div>
</template>
