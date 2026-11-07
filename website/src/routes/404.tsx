import { createFileRoute } from '@tanstack/solid-router'
import { NotFound } from '../components/NotFound'

export const Route = createFileRoute('/404')({
  head: () => ({ meta: [{ title: '找不到页面 · 肥牛音乐' }] }),
  component: NotFound,
})
