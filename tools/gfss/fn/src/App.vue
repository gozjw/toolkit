<template>
  <div class="transfer-container">
    <div class="info-card">
      <div class="theme-switch">
        <el-switch v-model="isDark" size="small" inline-prompt :active-icon="Moon" :inactive-icon="Sunny" />
      </div>

      <div class="info-grid">
        <div class="info-item">
          <span class="label">设备名称：</span>
          <span class="value">{{ hostname }}</span>
        </div>
        <div class="info-item">
          <span class="label">工作目录：</span>
          <span class="value">{{ workdir }}</span>
        </div>
      </div>
    </div>

    <div class="main-content">
      <div class="left-panel">
        <div class="text-section">
          <el-input v-model="plainText" type="textarea" :rows="isMobile ? 6 : 12" placeholder="传输文本，双击全选..." clearable
            @dblclick="selectAllText" ref="textRef" />
          <div class="btn-group">
            <el-button type="primary" :icon="Refresh" :loading="isRefreshing" @click="refresh">刷 新</el-button>
            <el-button type="success" :icon="Upload" @click="sendText">发 送</el-button>
          </div>
        </div>

        <div class="upload-section">
          <el-upload action="#" multiple :show-file-list="false" :before-upload="beforeUpload">
            <el-button type="warning">选择多文件</el-button>
          </el-upload>

          <el-button type="danger" @click="submitUpload" :icon="UploadFilled"
            :disabled="filesToUpload.length === 0 || isUploading">
            开始上传 {{ filesToUpload.length > 0 ? `(${filesToUpload.length}个文件)` : '' }}
          </el-button>
        </div>

        <div v-if="isUploading" class="progress-bar">
          <span class="progress-label">上传进度{{ remainTimeText }}</span>
          <el-progress :percentage="totalProgress" :stroke-width="18" text-inside />
        </div>

      </div>

      <div class="right-panel">
        <el-table ref="fileTableRef" :data="fileList" size="small" class="file-table" empty-text="无文件" border>
          <el-table-column type="index" label="序号" width="50" align="center" />
          <el-table-column min-width="180" prop="fileName" sortable :sort-method="sortFileName">
            <template #header>
              <span>文件列表 (共 {{ fileList.length }} 个文件)</span>
            </template>
            <template #default="scope">
              <el-link type="primary" :class="['file-name-link', { 'red-file-name': clickedFiles.has(scope.row) }]"
                @click="handleDownload(scope.row)" underline="never">
                {{ scope.row }}
              </el-link>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="60" align="center">
            <template #default="scope">
              <el-button type="danger" link @click="handleDelete(scope.row)">{{ delDesc }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

    </div>
  </div>
</template>

<script setup>
import { ref, nextTick, computed, onMounted, onUnmounted } from 'vue'
import { Sunny, Moon, Refresh, Upload, UploadFilled } from '@element-plus/icons-vue'
import { useDark } from '@vueuse/core'
import { createUniMsg } from '@/utils/unimsg'
import axios from 'axios'

const uniMsg = createUniMsg()

const isDark = useDark({
  // initialValue: 'light',
  storage: {
    getItem: () => null,
    setItem: () => { },
    removeItem: () => { },
  }
})

const isMobile = ref(false)

const checkDevice = () => {
  isMobile.value = window.innerWidth < 768
}

const hostname = ref('正在加载...')
const workdir = ref('正在加载...')
const delDesc = ref('删除')

const plainText = ref('')
const textRef = ref(null)
const isRefreshing = ref(false)

const fileTableRef = ref(null)
const fileList = ref([])
const clickedFiles = ref(new Set())

const filesToUpload = ref([])
const uploadProgresses = ref({})
const isUploading = ref(false)

const selectAllText = () => {
  if (textRef.value) {
    textRef.value.select()
  }
}

const fetchInfo = async () => {
  try {
    const res = await axios.get(`/info`)
    hostname.value = res.data[0] || '未知设备'
    workdir.value = res.data[1] || '未知目录'
    delDesc.value = res.data[2] || '删除'
  } catch (err) {
    ElMessage.error('无法获取系统信息')
  }
}

const fetchText = async () => {
  const res = await axios.get(`/text`)
  plainText.value = res.data || ''
}

const refresh = async () => {
  if (isRefreshing.value) return

  isRefreshing.value = true
  try {
    await Promise.all([
      fetchInfo(),
      fetchFileList(),
      fetchText()
    ])
    nextTick(() => {
      if (fileTableRef.value) {
        fileTableRef.value.clearSort()
      }
    })
  } catch (err) {
    ElMessage.error('刷新失败，请检查网络')
  } finally {
    isRefreshing.value = false
  }
}

const sendText = async () => {
  try {
    await axios.post(`/text`, plainText.value)
    ElMessage.success('发送成功')
  } catch (err) {
    let msg = `发送失败`
    if (err.response) {
      msg += `：${err.response.data}`;
    }
    ElMessage.error(msg)
  }
}

const fetchFileList = async () => {
  try {
    const res = await axios.get(`/list`)
    fileList.value = Array.isArray(res.data) ? res.data : []
  } catch (err) {
    let msg = `获取列表失败`
    if (err.response) {
      msg += `：${err.response.data}`;
    }
    ElMessage.error(msg)
  }
}

const sortFileName = (a, b) => {
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
};

// 用于计算速度、剩余时间
const lastLoadedTotal = ref(0)
const lastTimeStamp = ref(Date.now())
const speedBuffer = ref([]) // 速度缓冲区，平滑防抖
const remainSeconds = ref(0)
// 控制最低计算间隔，避免高频刷新
const MIN_CALC_INTERVAL = 150 // 毫秒，150ms只算一次
const MAX_SPEED_CACHE = 24 // 扩大缓存，平滑效果更强

const beforeUpload = (file) => {
  filesToUpload.value.push(file)
  uploadProgresses.value[file.uid] = 0
  return false
}

const totalProgress = computed(() => {
  if (filesToUpload.value.length === 0) return 0

  const totalBytes = filesToUpload.value.reduce((sum, file) => sum + file.size, 0)
  if (totalBytes === 0) return 0

  const loadedBytes = Object.values(uploadProgresses.value).reduce((a, b) => a + b, 0)

  return Math.round((loadedBytes / totalBytes) * 100)
})

// 格式化：秒 → XX分XX秒
const remainTimeText = computed(() => {
  const s = remainSeconds.value
  if (s <= 0) return ''
  const m = Math.floor(s / 60)
  const sec = s % 60
  const timeStr = m > 0 ? `${m}分${sec}秒` : `${sec}秒`
  return `，剩余时间：${timeStr}`
})

// 更新速度，剩余时间
function calcRemainTime() {
  const now = Date.now()
  const deltaMs = now - lastTimeStamp.value
  // 节流：不足150ms直接跳过，减少计算频率
  if (deltaMs < MIN_CALC_INTERVAL) return

  const deltaT = deltaMs / 1000
  if (deltaT <= 0) return

  // 全局总已上传字节
  const loadedBytes = Object.values(uploadProgresses.value).reduce((a, b) => a + b, 0)
  const deltaByte = loadedBytes - lastLoadedTotal.value
  const instantSpeed = deltaByte / deltaT

  // 过滤极端异常速度（瞬间爆冲的数值丢弃，防止时间断崖下跌）
  if (instantSpeed > 0) {
    speedBuffer.value.push(instantSpeed)
    if (speedBuffer.value.length > MAX_SPEED_CACHE) {
      speedBuffer.value.shift()
    }
  }

  // 平均速度
  const avgSpeed = speedBuffer.value.reduce((sum, val) => sum + val, 0) / speedBuffer.value.length
  const totalBytes = filesToUpload.value.reduce((sum, file) => sum + file.size, 0)
  const remainByte = totalBytes - loadedBytes

  if (avgSpeed > 0 && remainByte > 0) {
    remainSeconds.value = Math.round(remainByte / avgSpeed)
  } else {
    remainSeconds.value = 0
  }

  lastLoadedTotal.value = loadedBytes
  lastTimeStamp.value = now
}

// 并发上传多文件
const submitUpload = async () => {
  if (filesToUpload.value.length === 0) return
  isUploading.value = true

  // 上传前重置计时缓存
  lastLoadedTotal.value = 0
  lastTimeStamp.value = Date.now()
  speedBuffer.value = []
  remainSeconds.value = 0

  const uploadPromises = filesToUpload.value.map(file => {
    const formData = new FormData()
    formData.append('file', file)

    return axios.post(`/upload`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 0,
      onUploadProgress: (progressEvent) => {
        uploadProgresses.value[file.uid] = progressEvent.loaded
        // 每一段进度更新，计算剩余时间
        calcRemainTime()
      }
    })
  })

  try {
    await Promise.all(uploadPromises)
    ElMessage.success('上传成功')
    fetchFileList()
  } catch (err) {
    let msg = `上传失败`
    if (err.response) {
      msg += `：${err.response.data}`;
    }
    ElMessage.error(msg)
  } finally {
    isUploading.value = false
    filesToUpload.value = []
    uploadProgresses.value = {}
    speedBuffer.value = []
    remainSeconds.value = 0
  }
}

const handleDownload = (filename) => {
  clickedFiles.value.add(filename)
  const downloadUrl = `/dl/${encodeURIComponent(filename)}`
  const link = document.createElement('a')
  link.href = downloadUrl
  link.setAttribute('download', filename)
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

const handleDelete = async (filename) => {
  try {
    await axios.delete(`/${encodeURIComponent(filename)}`)
    uniMsg.success(`${delDesc.value}: ${filename}`)
    fetchFileList()
  } catch (err) {
    let msg = `${delDesc.value}失败: ${filename}`
    if (err.response) {
      msg += ` ${err.response.data}`;
    }
    ElMessage.error(msg)
  }
}

onMounted(() => {
  refresh()
  checkDevice()
  window.addEventListener('resize', checkDevice)
})

onUnmounted(() => {
  window.removeEventListener('resize', checkDevice)
})
</script>

<style scoped>
.transfer-container {
  max-width: 1000px;
  margin: 0 auto;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-sizing: border-box;
}

.info-card {
  position: relative;
  background-color: var(--el-fill-color-light);
  border-radius: 6px;
  padding: 10px 14px;
  font-size: 13px;
}

.theme-switch {
  position: absolute;
  top: 6px;
  right: 14px;
  z-index: 10;
}

.info-grid {
  display: flex;
  flex-direction: row;
  gap: 20px;
  padding-right: 60px;
}

.info-item {
  flex: 1;
  display: flex;
  min-width: 0;
}

.label {
  font-weight: bold;
  color: var(--el-text-color-regular);
  white-space: nowrap;
  user-select: none;
}

.value {
  color: var(--el-text-color-primary);
  word-break: break-all;
}

.main-content {
  display: flex;
  gap: 16px;
  width: 100%;
}

.left-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.right-panel {
  flex: 1;
  min-width: 0;
}

.text-section,
.upload-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

:deep(.el-textarea__inner::placeholder) {
  user-select: none;
}

.btn-group,
.upload-section {
  display: flex;
  flex-direction: row;
  gap: 8px;
}

.btn-group .el-button,
.upload-section .el-button,
.upload-section :deep(.el-upload) {
  flex: 1;
}

.upload-section :deep(.el-upload) .el-button {
  width: 100%;
}

.progress-bar {
  background-color: var(--el-fill-color-blank);
  border: 1px solid var(--el-border-color);
  padding: 8px;
  border-radius: 6px;
}

.progress-label {
  font-size: 12px;
  display: block;
  margin-bottom: 4px;
}

.file-table {
  width: 100%;
}

:deep(.el-table__empty-text),
:deep(.el-table th.el-table__cell) {
  user-select: none;
}

.file-name-link {
  display: inline-block;
  max-width: 100%;
  text-align: left;
  white-space: normal !important;
  word-break: break-all !important;
  line-height: 1.4;
  padding: 4px 0;
  cursor: pointer;
}

.red-file-name {
  color: var(--el-color-danger) !important;
}

@media (max-width: 768px) {
  .info-grid {
    flex-direction: column;
    gap: 6px;
  }

  .main-content {
    flex-direction: column;
    gap: 16px;
  }

  .left-panel,
  .right-panel {
    flex: none;
    width: 100%;
  }
}
</style>
