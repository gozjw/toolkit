<template>
  <div class="form-container">
    <el-card class="form-card">
      <div class="header-container">
        <el-switch class="theme-switch-wrapper" v-model="isDark" size="small" inline-prompt :active-icon="Moon"
          :inactive-icon="Sunny" />
      </div>
      <el-form ref="formRef" :model="formData" :rules="rules" :hide-required-asterisk="true" label-position="top">
        <el-form-item label="文件" prop="fileName">
          <input class="hidden-input" ref="fileInputRef" type="file" accept=".secb" @change="handleFileChange" />
          <el-input v-model="formData.fileName" readonly placeholder="选择文件" @click="triggerFileInput"
            class="cursor-pointer">
            <template #prefix>
              <el-icon>
                <Document />
              </el-icon>
            </template>
          </el-input>
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input v-model="formData.password" type="password" @keyup.enter="handleDecode" placeholder="用于加密或解密"
            clearable show-password />
        </el-form-item>

        <el-form-item label="确认密码" prop="confirmPassword">
          <el-input v-model="formData.confirmPassword" type="password" placeholder="用于加密" clearable />
        </el-form-item>

        <div class="button-group">
          <el-button class="flex-1" type="primary" @click="handleDecode">解 密</el-button>
          <el-button class="flex-1" type="success" @click="handleDownload">加密并下载</el-button>
        </div>

        <el-form-item label="选项" prop="label">
          <el-select class="w-full" v-model="formData.label" allow-create filterable no-data-text="无数据"
            placeholder="例如: 账号、密码、记事等" @change="handleLabelChange">
            <el-option v-for="item in options" :key="item.label" :label="item.label" :value="item.label" />
          </el-select>
        </el-form-item>

        <el-form-item label="内容（双击全选）" prop="value">
          <el-badge class="w-full" is-dot :hidden="formData.valueDotHidden">
            <el-input v-model="formData.value" type="textarea" :rows="7" clearable placeholder="请输入内容..."
              @input="handleValueInput" @dblclick="handleDblClickValue" />
          </el-badge>
        </el-form-item>

        <div class="button-group">
          <el-button class="flex-1" type="primary" @click="handleRefreshValue">刷 新</el-button>
          <el-button class="flex-1" type="primary" @click="handleAddOrModify">添加/修改</el-button>
          <el-button class="flex-1" type="danger" @click="handleDelete">删 除</el-button>
        </div>
      </el-form>
      <footer>Argon2id · AES-256-GCM · 本地加密</footer>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useDark } from '@vueuse/core'
import { Document } from '@element-plus/icons-vue'
import { Sunny, Moon } from '@element-plus/icons-vue'
import { encryptData, decryptData } from '@/utils/cryptoService';

const isDark = useDark({
  storage: {
    getItem: () => null,
    setItem: () => { },
    removeItem: () => { },
  }
})

const formRef = ref(null)
const fileInputRef = ref(null)

const options = ref([])

const clearOptions = () => {
  options.value = []
  formData.label = ""
  formData.value = ""
}

const formData = reactive({
  fileRaw: null,
  fileName: '',
  password: '',
  confirmPassword: '',
  label: '',
  value: '',
  valueDotHidden: true,
})

const validateConfirmPassword = (rule, value, callback) => {
  if (value === "") {
    callback(new Error('确认密码不能为空'))
  } else if (formData.password !== value) {
    callback(new Error('确认密码不一致'))
  } else {
    callback()
  }
}

const rules = {
  fileName: [
    { required: true, message: '文件不能为空', trigger: 'submit' }
  ],
  password: [
    { required: true, message: '密码不能为空', trigger: 'submit' }
  ],
  confirmPassword: [
    { validator: validateConfirmPassword, trigger: 'submit' }
  ]
}

const sortOptions = () => {
  options.value.sort((a, b) => a.label.localeCompare(b.label, 'zh-CN'))
}

const handleLabelChange = (value) => {
  const item = options.value.find(item => item.label === value)
  if (item) {
    formData.value = item.value
  } else {
    formData.value = ""
  }
  formData.valueDotHidden = true
}

const handleRefreshValue = () => {
  const item = options.value.find(item => item.label === formData.label)
  if (item) {
    formData.value = item.value
    formData.valueDotHidden = true
  }
}

const handleAddOrModify = () => {
  if (!formData.label || !formData.value) return

  if (options.value.find(item => item.label === formData.label)) {
    handleModify()
  } else {
    handleAdd()
  }
}

const handleAdd = () => {
  options.value.push({
    label: formData.label,
    value: formData.value
  });
  sortOptions()
  formData.valueDotHidden = true
  ElMessage.success("添加成功")
}

const handleModify = () => {
  if (formData.label) {
    ElMessageBox.confirm('确认修改：' + formData.label, '', {
      cancelButtonText: "取 消",
      confirmButtonText: '确 认',
      type: 'warning',
    }).then(() => {
      const item = options.value.find(item => item.label === formData.label)
      item.value = formData.value
      formData.valueDotHidden = true
      ElMessage.success("修改成功")
    })
  }
}

const handleDelete = () => {
  if (formData.label) {
    ElMessageBox.confirm('确认删除：' + formData.label, '', {
      cancelButtonText: "取 消",
      confirmButtonText: '确 认',
      type: 'warning',
    }).then(() => {
      options.value = options.value.filter(item => item.label !== formData.label)
      if (options.value.length) {
        formData.label = options.value[0].label
        formData.value = ""
      } else {
        formData.label = ""
        formData.value = ""
      }
      formData.valueDotHidden = true
      ElMessage.success("删除成功")
    })
  }
}

const handleValueInput = (value) => {
  if (!value.length) {
    formData.valueDotHidden = true
    return
  }
  formData.valueDotHidden = false
}

const handleDblClickValue = (event) => {
  event.target.select()
}

const triggerFileInput = () => { fileInputRef.value?.click() }

const handleFileChange = (event) => {
  const files = event.target.files
  if (files && files.length > 0) {
    formData.fileRaw = files[0]
    formData.fileName = files[0].name
  }
  clearOptions()
}

const handleDecode = async () => {
  if (!formRef.value) return

  formRef.value.validateField(['fileName', 'password'], async (isValid) => {
    if (isValid) {
      try {
        if (formData.fileRaw) {
          const encryptedContent = await readFileAsText(formData.fileRaw)
          const jsonString = await decryptData(encryptedContent, formData.password)
          options.value = JSON.parse(jsonString)
          sortOptions()
          if (options.value.length) {
            formData.label = options.value[0].label
            formData.value = ""
          } else {
            formData.label = ""
            formData.value = ""
          }
          ElMessage.success('解密成功')
        }
      } catch (error) {
        clearOptions()
        console.error(error)
        ElMessage.error('密码或文件错误')
      }
    } else {
      setTimeout(() => {
        formRef.value.clearValidate()
      }, 3000)
    }
  })
}

const handleDownload = async () => {
  if (!options.value.length) return
  if (!formRef.value) return

  formRef.value.validateField(['password', 'confirmPassword'], async (isValid) => {
    if (isValid) {
      try {
        const jsonString = JSON.stringify(options.value)
        const encryptedBase64Result = await encryptData(jsonString, formData.password)
        const blob = new Blob([encryptedBase64Result], { type: 'application/octet-stream' })
        const downloadUrl = URL.createObjectURL(blob)
        const downloadLink = document.createElement('a')
        downloadLink.href = downloadUrl
        downloadLink.download = newFileName()
        document.body.appendChild(downloadLink)
        downloadLink.click()
        document.body.removeChild(downloadLink)
        URL.revokeObjectURL(downloadUrl)
      } catch (error) {
        console.error(error)
        ElMessage.error('加密并下载出现未知错误')
      }
    } else {
      setTimeout(() => {
        formRef.value.clearValidate()
      }, 3000)
    }
  })
}

const newFileName = () => {
  const now = new Date()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const date = String(now.getDate()).padStart(2, '0')
  const hours = String(now.getHours()).padStart(2, '0')
  const minutes = String(now.getMinutes()).padStart(2, '0')
  const timeStamp = `${month}${date}${hours}${minutes}`

  let cleanName = formData.fileName.trim()

  if (!cleanName) {
    return `${timeStamp}.secb`
  }

  const timeRegex = /(_)?(\d{8})(?=\.secb$)/
  if (timeRegex.test(cleanName)) {
    return cleanName.replace(timeRegex, (match, p1) => {
      return p1 ? `_${timeStamp}` : timeStamp
    })
  } else {
    const lastIndex = cleanName.lastIndexOf('.secb')
    const prefix = cleanName.substring(0, lastIndex)
    return `${prefix}_${timeStamp}.secb`
  }
}

// 读取本地文本文件
const readFileAsText = (file) => {
  return new Promise((resolve) => {
    const reader = new FileReader()
    reader.onload = (e) => resolve(e.target.result)
    reader.readAsText(file)
  })
}
</script>

<style scoped>
.header-container {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  width: 100%;
}

.theme-switch-wrapper {
  width: fit-content;
}

.hidden-input {
  display: none;
}

.cursor-pointer :deep(.el-input__inner) {
  cursor: pointer;
}

.form-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background-color: var(--el-bg-color-page);
  padding: 16px;
  box-sizing: border-box;
}

.form-card {
  width: 100%;
  max-width: 500px;
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.w-full {
  width: 100%;
}

.button-group {
  display: flex;
  gap: 16px;
  margin-bottom: 18px;
}

.flex-1 {
  flex: 1;
}

footer {
  text-align: center;
  font-size: 0.75rem;
  color: #6b7280;
}

@media (max-width: 480px) {
  .form-container {
    padding: 8px;
    align-items: flex-start;
  }

  .form-card {
    border-radius: 8px;
    border: none;
  }
}
</style>
