import type { CSSProperties, ReactNode } from 'react'
import { useTitleLang } from '../context/TitleLangContext'
import { colorForTitle, initialForTitle } from '../lib/util'

interface ThumbItem {
  title: string
  titleEn?: string | null
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
  const { displayTitle } = useTitleLang()
  const style: CSSProperties = { ...styleProp }
  if (width != null) style.width = width
  if (height != null) style.height = height
  if (fontSize != null) style.fontSize = fontSize + 'px'

  if (item.coverImageUrl) {
    style.backgroundImage = `url("${item.coverImageUrl}")`
  } else {
    // 色は表示言語で変わらないよう title から決め、頭文字だけ表示中の作品名に合わせる
    style.background = colorForTitle(item.title)
  }

  return (
    <div className={className} style={style} aria-hidden="true">
      {!item.coverImageUrl && initialForTitle(displayTitle(item))}
      {children}
    </div>
  )
}
