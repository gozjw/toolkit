import { ElMessage } from 'element-plus'

export const createUniMsg = () => {
  let inst = null

  const unimsg = (options) => {
    if (inst) inst.close()
    const config = typeof options === 'string' ? { message: options } : options
    inst = ElMessage(config)
    return inst
  };
  
  ['success', 'warning', 'info', 'error'].forEach(type => {
    unimsg[type] = (options) => {
      if (inst) inst.close()
      const config = typeof options === 'string' ? { message: options } : options
      inst = ElMessage({ ...config, type })
      return inst
    }
  })

  return unimsg
}
