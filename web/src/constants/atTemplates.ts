export type ATTemplateItem = {
  label: string
  value: string
}

export type ATTemplateGroup = {
  label: string
  items: ATTemplateItem[]
}

/** 内置默认模板（用户可通过 AT 终端的「编辑模板」按钮覆盖保存到 localStorage） */
export const DEFAULT_AT_TEMPLATES: ATTemplateGroup[] = [
  {
    label: '基础命令',
    items: [
      { label: '连通性 (AT)', value: 'AT' },
      { label: '模块信息 (ATI)', value: 'ATI' },
      { label: '固件版本 (AT+QGMR)', value: 'AT+QGMR' },
      { label: '固件信息 (AT+GMR)', value: 'AT+GMR' },
      { label: '模块温度 (AT+QTEMP)', value: 'AT+QTEMP' },
      { label: 'IMEI (AT+CGSN)', value: 'AT+CGSN' },
      { label: 'ICCID (AT+QCCID)', value: 'AT+QCCID' },
      { label: 'IMSI (AT+CIMI)', value: 'AT+CIMI' },
      { label: '信号 (AT+CSQ)', value: 'AT+CSQ' },
      { label: '运营商 (AT+COPS?)', value: 'AT+COPS?' },
      { label: '运营商名 (AT+QSPN)', value: 'AT+QSPN' },
      { label: '网络信息 (AT+QNWINFO)', value: 'AT+QNWINFO' },
      { label: '小区信息 (AT+QENG="servingcell")', value: 'AT+QENG="servingcell"' },
      { label: '天线信号 (AT+QRSRP)', value: 'AT+QRSRP' },
      { label: '注册状态 (AT+CREG?)', value: 'AT+CREG?' },
      { label: '查询APN (AT+CGDCONT?)', value: 'AT+CGDCONT?' },
      { label: '更改IMEI (AT+EGMR=1,7,"xxx")', value: 'AT+EGMR=1,7,"xxx"' },
      { label: '禁用SIM卡检测 (AT+QSIMDET=0,0)', value: 'AT+QSIMDET=0,0' },
    ]
  },
  {
    label: 'USBNET / 模式',
    items: [
      { label: '查询模块模式 (AT+QCFG="usbnet")', value: 'AT+QCFG="usbnet"' },
      { label: '设置QMI模式 (AT+QCFG="usbnet",0)', value: 'AT+QCFG="usbnet",0' },
      { label: '设置ECM模式 (AT+QCFG="usbnet",1)', value: 'AT+QCFG="usbnet",1' },
      { label: '设置MBIM模式 (AT+QCFG="usbnet",2)', value: 'AT+QCFG="usbnet",2' },
      { label: '设置RNDIS模式 (AT+QCFG="usbnet",3)', value: 'AT+QCFG="usbnet",3' },
    ]
  },
  {
    label: '网络控制',
    items: [
      { label: '飞行模式 ON (AT+CFUN=0)', value: 'AT+CFUN=0' },
      { label: '飞行模式 OFF (AT+CFUN=1)', value: 'AT+CFUN=1' },
      { label: '重启模组 (AT+CFUN=1,1)', value: 'AT+CFUN=1,1' },
      { label: '附着状态 (AT+CGATT?)', value: 'AT+CGATT?' },
      { label: '脱附 (AT+CGATT=0)', value: 'AT+CGATT=0' },
      { label: '附着 (AT+CGATT=1)', value: 'AT+CGATT=1' },
    ]
  },
  {
    label: 'ECM/RNDIS 模式',
    items: [
      { label: '查询IPPT NAT模式 (AT+QMAP="IPPT_NAT")', value: 'AT+QMAP="IPPT_NAT"' },
      { label: '关闭IPPT NAT (AT+QMAP="IPPT_NAT",0)', value: 'AT+QMAP="IPPT_NAT",0' },
      { label: '查询VLAN (AT+QMAP="VLAN")', value: 'AT+QMAP="VLAN"' },
      { label: '查询MPDN规则 (AT+QMAP="MPDN_rule")', value: 'AT+QMAP="MPDN_rule"' },
      { label: '清除MPDN规则 (AT+QMAP="MPDN_rule",0)', value: 'AT+QMAP="MPDN_rule",0' },
      { label: '设置MPDN规则', value: 'AT+QMAP="MPDN_rule",0,1,0,3,1,"FF:FF:FF:FF:FF:FF"' },
      { label: '查询DHCPV4DNS (AT+QMAP="DHCPV4DNS")', value: 'AT+QMAP="DHCPV4DNS"' },
      { label: '开启DHCPV4DNS', value: 'AT+QMAP="DHCPV4DNS","enable"' },
      { label: '关闭DHCPV4DNS', value: 'AT+QMAP="DHCPV4DNS","disable"' },
      { label: '查询DHCPV6DNS (AT+QMAP="DHCPV6DNS")', value: 'AT+QMAP="DHCPV6DNS"' },
      { label: '开启DHCPV6DNS', value: 'AT+QMAP="DHCPV6DNS","enable"' },
      { label: '关闭DHCPV6DNS', value: 'AT+QMAP="DHCPV6DNS","disable"' },
    ]
  },
  {
    label: '3/4/5G 网络配置',
    items: [
      { label: '切卡1 (AT+QUIMSLOT=1)', value: 'AT+QUIMSLOT=1' },
      { label: '切卡2 (AT+QUIMSLOT=2)', value: 'AT+QUIMSLOT=2' },
      { label: '修改APN (IPV4V6)', value: 'AT+CGDCONT=1,"IPV4V6","CMNET"' },
      { label: '查询WCDMA频段', value: 'AT+QNWPREFCFG="gw_band"' },
      { label: '查询LTE频段', value: 'AT+QNWPREFCFG="lte_band"' },
      { label: '查询5GNR NSA频段', value: 'AT+QNWPREFCFG="nsa_nr5g_band"' },
      { label: '查询5GNR SA频段', value: 'AT+QNWPREFCFG="nr5g_band"' },
      { label: '查询搜网模式', value: 'AT+QNWPREFCFG="mode_pref"' },
      { label: '切换AUTO模式', value: 'AT+QNWPREFCFG="mode_pref",AUTO' },
      { label: '切换仅3G模式', value: 'AT+QNWPREFCFG="mode_pref",WCDMA' },
      { label: '切换仅4G模式', value: 'AT+QNWPREFCFG="mode_pref",LTE' },
      { label: '切换仅5G模式', value: 'AT+QNWPREFCFG="mode_pref",NR5G' },
      { label: '切换5G+4G模式', value: 'AT+QNWPREFCFG="mode_pref",NR5G:LTE' },
      { label: '查询SA/NSA状态', value: 'AT+QNWPREFCFG="nr5g_disable_mode"' },
      { label: 'SA+NSA同时使用', value: 'AT+QNWPREFCFG="nr5g_disable_mode",0' },
      { label: '仅NSA网络', value: 'AT+QNWPREFCFG="nr5g_disable_mode",1' },
      { label: '仅SA网络', value: 'AT+QNWPREFCFG="nr5g_disable_mode",2' },
      { label: '查询基站锁定', value: 'AT+QNWLOCK="common/5g"' },
      { label: '锁定基站示例', value: 'AT+QNWLOCK="common/5g",214,524910,30,41' },
      { label: '解锁基站锁定', value: 'AT+QNWLOCK="common/5g",0' },
    ]
  },
  {
    label: '漫游服务',
    items: [
      { label: '关闭漫游', value: 'AT+QCFG="roamservice",1,1' },
      { label: '恢复自动', value: 'AT+QCFG="roamservice",255,1' },
    ]
  },
  {
    label: '短信 / USSD',
    items: [
      { label: '列出短信 (AT+CMGL=4)', value: 'AT+CMGL=4' },
      { label: '读取短信 (AT+CMGR=1)', value: 'AT+CMGR=1' },
      { label: '删除所有短信 (AT+CMGD=1,4)', value: 'AT+CMGD=1,4' },
      { label: 'USSD 示例 (AT+CUSD=1,"*100#",15)', value: 'AT+CUSD=1,"*100#",15' },
    ]
  },
]

const STORAGE_KEY = 'vohive_at_templates'

/** 从 localStorage 读取用户自定义模板，不存在则返回默认模板 */
export function loadATTemplates(): ATTemplateGroup[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return DEFAULT_AT_TEMPLATES
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.length > 0) return parsed as ATTemplateGroup[]
  } catch { /* ignore corrupt data */ }
  return DEFAULT_AT_TEMPLATES
}

/** 保存用户自定义模板到 localStorage */
export function saveATTemplates(groups: ATTemplateGroup[]): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(groups))
}

/** 重置为默认模板 */
export function resetATTemplates(): ATTemplateGroup[] {
  localStorage.removeItem(STORAGE_KEY)
  return DEFAULT_AT_TEMPLATES
}
