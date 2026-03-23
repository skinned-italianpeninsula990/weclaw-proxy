import { useState, useEffect, useCallback } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { fetchRoutes, updateRoutes, fetchAdapters, fetchSmartRouting, updateSmartRouting } from '@/lib/api'

// 路由规则类型
interface RouteRule {
  match: {
    prefix?: string
    user_ids?: string[]
  }
  adapter: string
}

interface RoutingConfig {
  default_adapter: string
  rules: RouteRule[]
}

interface SmartRoutingConfig {
  enabled: boolean
  api_key: string
  base_url: string
  model: string
  temperature: number
}

export function RoutesPage() {
  const [routing, setRouting] = useState<RoutingConfig>({ default_adapter: '', rules: [] })
  const [smartRouting, setSmartRouting] = useState<SmartRoutingConfig>({
    enabled: false, api_key: '', base_url: '', model: '', temperature: 0.1,
  })
  const [adapterNames, setAdapterNames] = useState<string[]>([])
  const [newRule, setNewRule] = useState({ prefix: '', adapter: '' })
  const [saving, setSaving] = useState(false)
  const [savingSmart, setSavingSmart] = useState(false)

  const load = useCallback(async () => {
    try {
      const [routeData, adapterData, smartData] = await Promise.all([
        fetchRoutes(), fetchAdapters(), fetchSmartRouting(),
      ])
      setRouting({
        default_adapter: routeData?.default_adapter || '',
        rules: routeData?.rules || [],
      })
      setAdapterNames((adapterData || []).map((a: { name: string }) => a.name))
      if (smartData) {
        setSmartRouting({
          enabled: smartData.enabled || false,
          api_key: smartData.api_key || '',
          base_url: smartData.base_url || '',
          model: smartData.model || '',
          temperature: smartData.temperature || 0.1,
        })
      }
    } catch {
      // 忽略
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  // 保存路由配置
  const handleSave = async () => {
    setSaving(true)
    try {
      await updateRoutes(routing as unknown as Record<string, unknown>)
    } finally {
      setSaving(false)
    }
  }

  // 保存智能路由配置
  const handleSaveSmart = async () => {
    setSavingSmart(true)
    try {
      await updateSmartRouting(smartRouting as unknown as Record<string, unknown>)
    } finally {
      setSavingSmart(false)
    }
  }

  // 添加路由规则
  const handleAddRule = () => {
    if (!newRule.prefix.trim() || !newRule.adapter) return
    setRouting({
      ...routing,
      rules: [...routing.rules, {
        match: { prefix: newRule.prefix.trim() },
        adapter: newRule.adapter,
      }],
    })
    setNewRule({ prefix: '', adapter: '' })
  }

  // 删除路由规则
  const handleDeleteRule = (index: number) => {
    setRouting({
      ...routing,
      rules: routing.rules.filter((_, i) => i !== index),
    })
  }

  return (
    <div className="space-y-6">
      {/* 智能路由 */}
      <Card>
        <CardHeader>
          <CardTitle>智能路由</CardTitle>
          <CardDescription>
            使用 LLM 自动分析消息意图，路由到最合适的 Agent。优先级：前缀规则 &gt; 智能路由 &gt; 默认 Agent
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label htmlFor="smart-toggle">启用智能路由</Label>
              <p className="text-sm text-muted-foreground">
                开启后，无前缀的消息将由小模型自动分类
              </p>
            </div>
            <Switch
              id="smart-toggle"
              checked={smartRouting.enabled}
              onCheckedChange={(checked) => setSmartRouting({ ...smartRouting, enabled: checked })}
            />
          </div>

          {smartRouting.enabled && (
            <>
              <Separator />
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="smart-base-url">API 地址</Label>
                  <Input
                    id="smart-base-url"
                    value={smartRouting.base_url}
                    onChange={e => setSmartRouting({ ...smartRouting, base_url: e.target.value })}
                    placeholder="https://api.openai.com/v1"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="smart-api-key">API Key</Label>
                  <Input
                    id="smart-api-key"
                    value={smartRouting.api_key}
                    onChange={e => setSmartRouting({ ...smartRouting, api_key: e.target.value })}
                    placeholder="sk-..."
                    type="password"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="smart-model">模型</Label>
                  <Input
                    id="smart-model"
                    value={smartRouting.model}
                    onChange={e => setSmartRouting({ ...smartRouting, model: e.target.value })}
                    placeholder="gpt-4o-mini"
                  />
                </div>
              </div>
              <details className="mt-4">
                <summary className="text-sm text-muted-foreground cursor-pointer hover:text-foreground transition-colors">
                  高级设置
                </summary>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-3">
                  <div className="space-y-2">
                    <Label htmlFor="smart-temp">温度（Temperature）</Label>
                    <p className="text-xs text-muted-foreground">值越低路由判断越稳定，推荐 0.1</p>
                    <Input
                      id="smart-temp"
                      type="number"
                      step="0.1"
                      min="0"
                      max="2"
                      value={smartRouting.temperature}
                      onChange={e => setSmartRouting({ ...smartRouting, temperature: parseFloat(e.target.value) || 0 })}
                    />
                  </div>
                </div>
              </details>
            </>
          )}

          <div className="flex justify-end">
            <Button onClick={handleSaveSmart} disabled={savingSmart}>
              {savingSmart ? '保存中...' : '保存智能路由'}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 默认 Agent */}
      <Card>
        <CardHeader>
          <CardTitle>默认 Agent</CardTitle>
          <CardDescription>
            当消息不匹配任何路由规则时，使用此 Agent 处理
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-4">
            <Select
              value={routing.default_adapter || undefined}
              onValueChange={(v) => { if (v) setRouting({ ...routing, default_adapter: v }) }}
            >
              <SelectTrigger className="w-64">
                <SelectValue placeholder="选择默认 Agent" />
              </SelectTrigger>
              <SelectContent>
                {adapterNames.map(name => (
                  <SelectItem key={name} value={name}>{name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button onClick={handleSave} disabled={saving}>
              {saving ? '保存中...' : '保存'}
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* 路由规则 */}
      <Card>
        <CardHeader>
          <CardTitle>前缀路由规则</CardTitle>
          <CardDescription>
            按消息前缀匹配路由到不同的 Agent，规则按顺序优先匹配
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* 规则列表 */}
          {routing.rules.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12">#</TableHead>
                  <TableHead>匹配前缀</TableHead>
                  <TableHead>目标 Agent</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {routing.rules.map((rule, i) => (
                  <TableRow key={i}>
                    <TableCell className="text-muted-foreground">{i + 1}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="font-mono">
                        {rule.match.prefix || '(无前缀)'}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-medium">{rule.adapter}</TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() => handleDeleteRule(i)}
                      >
                        删除
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="text-center py-8 text-muted-foreground">
              <p>暂无路由规则，所有消息将发送到默认 Agent</p>
            </div>
          )}

          <Separator />

          {/* 添加新规则 */}
          <div className="flex items-end gap-4">
            <div className="space-y-2 flex-1">
              <Label htmlFor="rule-prefix">消息前缀</Label>
              <Input
                id="rule-prefix"
                value={newRule.prefix}
                onChange={e => setNewRule({ ...newRule, prefix: e.target.value })}
                placeholder="例如: /画图"
              />
            </div>
            <div className="space-y-2 w-48">
              <Label>目标 Agent</Label>
              <Select
                value={newRule.adapter}
                onValueChange={(v) => { if (v) setNewRule({ ...newRule, adapter: v }) }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择 Agent" />
                </SelectTrigger>
                <SelectContent>
                  {adapterNames.map(name => (
                    <SelectItem key={name} value={name}>{name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button onClick={handleAddRule} variant="secondary">
              添加规则
            </Button>
          </div>

          {routing.rules.length > 0 && (
            <div className="flex justify-end">
              <Button onClick={handleSave} disabled={saving}>
                {saving ? '保存中...' : '保存路由规则'}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
