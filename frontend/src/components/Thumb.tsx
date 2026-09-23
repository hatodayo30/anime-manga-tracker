import type { CSSProperties, ReactNode } from 'react'
import { colorForTitle, initialForTitle } from '../lib/util'

interface ThumbItem {
  title: string
  coverImageUrl?: string | null
}

export function Thumb({
  item,
  width,
  height,
  fontSize,
  className = 'thumb',
  style: styleProp,
  children,
}: {
  item: ThumbItem
  width?: string
  height?: string
  fontSize?: number
  className?: string
  style?: CSSProperties
  children?: ReactNode
}) {
  const style: CSSProperties = { ...styleProp }
  if (width != null) style.width = width
  if (height != null) style.height = height
  if (fontSize != null) style.fontSize = fontSize + 'px'

  if (item.coverImageUrl) {
    style.backgroundImage = `url("${item.coverImageUrl}")`
  } else {
    style.background = colorForTitle(item.title)
  }

  return (
    <div className={className} style={style}>
      {!item.coverImageUrl && initialForTitle(item.title)}
      {children}
    </div>
  )
}
