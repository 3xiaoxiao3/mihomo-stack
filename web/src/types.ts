export interface UpdateRecord {
  id: string
  trigger: string
  stage: string
  success: boolean
  rolled_back: boolean
  message?: string
  started_at: string
  finished_at: string
}

export interface Status {
  guardian_version: string
  mihomo: {
    online: boolean
    version: string
  }
  update_busy: boolean
  scheduler_enabled: boolean
  update_interval: string
  source_count: number
  dashboard_url: string
  last_update: UpdateRecord | null
}

export interface Backup {
  id: string
  size: number
  created_at: string
}

export interface Diagnostics {
  guardian: {
    version: string
    go: string
    os: string
    arch: string
  }
  authentication_enabled: boolean
  scheduler_enabled: boolean
  active_config: {
    exists: boolean
    size?: number
    modified_at?: string
  }
  mihomo_reachable: boolean
}
