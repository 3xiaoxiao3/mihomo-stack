<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError } from './api'
import DashboardView from './components/DashboardView.vue'
import LoginView from './components/LoginView.vue'
import type { Backup, Diagnostics, Status, UpdateRecord } from './types'

const authenticated = ref<boolean | null>(null)
const status = ref<Status | null>(null)
const history = ref<UpdateRecord[]>([])
const backups = ref<Backup[]>([])
const diagnostics = ref<Diagnostics | null>(null)
const loading = ref(false)
const loginLoading = ref(false)
const action = ref<'update' | 'restore' | null>(null)
const error = ref('')

async function loadDashboard(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [nextStatus, nextHistory, nextBackups, nextDiagnostics] = await Promise.all([
      api.status(),
      api.history(),
      api.backups(),
      api.diagnostics(),
    ])
    status.value = nextStatus
    history.value = nextHistory
    backups.value = nextBackups
    diagnostics.value = nextDiagnostics
    authenticated.value = true
  } catch (caught) {
    if (caught instanceof ApiError && caught.status === 401) {
      authenticated.value = false
    } else {
      error.value = errorMessage(caught)
      authenticated.value = false
    }
  } finally {
    loading.value = false
  }
}

async function login(token: string): Promise<void> {
  error.value = ''
  loginLoading.value = true
  try {
    await api.login(token)
    authenticated.value = true
    await loadDashboard()
  } catch (caught) {
    error.value = errorMessage(caught)
  } finally {
    loginLoading.value = false
  }
}

async function logout(): Promise<void> {
  try {
    await api.logout()
  } finally {
    authenticated.value = false
    status.value = null
  }
}

async function update(): Promise<void> {
  action.value = 'update'
  error.value = ''
  try {
    await api.update()
    await loadDashboard()
  } catch (caught) {
    const message = errorMessage(caught)
    await loadDashboard()
    error.value = message
  } finally {
    action.value = null
  }
}

async function restore(id: string): Promise<void> {
  action.value = 'restore'
  error.value = ''
  try {
    await api.restore(id)
    await loadDashboard()
  } catch (caught) {
    const message = errorMessage(caught)
    await loadDashboard()
    error.value = message
  } finally {
    action.value = null
  }
}

function errorMessage(caught: unknown): string {
  return caught instanceof Error ? caught.message : '发生未知错误'
}

onMounted(loadDashboard)
</script>

<template>
  <main v-if="authenticated === null" class="centered" aria-live="polite">
    <div class="loader" />
    <p>正在连接 Guardian…</p>
  </main>
  <LoginView
    v-else-if="!authenticated"
    :external-error="error"
    :submitting="loginLoading"
    @login="login"
  />
  <DashboardView
    v-else
    :status="status"
    :history="history"
    :backups="backups"
    :diagnostics="diagnostics"
    :loading="loading"
    :action="action"
    :error="error"
    @refresh="loadDashboard"
    @update="update"
    @restore="restore"
    @logout="logout"
  />
</template>
