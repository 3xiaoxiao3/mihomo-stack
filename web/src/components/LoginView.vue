<script setup lang="ts">
import { ref } from 'vue'

defineProps<{ externalError: string; submitting: boolean }>()
const emit = defineEmits<{ login: [token: string] }>()

const token = ref('')

function submit(): void {
  if (!token.value) return
  emit('login', token.value)
}
</script>

<template>
  <main class="login-shell">
    <section class="login-card" aria-labelledby="login-title">
      <div class="brand-mark" aria-hidden="true">M</div>
      <p class="eyebrow">MIHOMO STACK</p>
      <h1 id="login-title">安全配置控制台</h1>
      <p class="muted">输入部署时生成的管理员令牌。令牌只发送一次，不会保存在浏览器存储中。</p>
      <form @submit.prevent="submit">
        <label for="token">管理员令牌</label>
        <input
          id="token"
          v-model="token"
          name="token"
          type="password"
          autocomplete="current-password"
          minlength="16"
          required
          autofocus
        />
        <p v-if="externalError" class="alert error" role="alert">{{ externalError }}</p>
        <button class="primary full" type="submit" :disabled="submitting || token.length < 16">
          {{ submitting ? '正在验证…' : '进入控制台' }}
        </button>
      </form>
    </section>
  </main>
</template>
