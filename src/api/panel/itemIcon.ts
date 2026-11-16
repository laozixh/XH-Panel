import { post } from '@/utils/request'

export function addMultiple<T>(req: Panel.ItemInfo[]) {
  return post<T>({
    url: '/panel/itemIcon/addMultiple',
    data: req,
  })
}

export function edit<T>(req: Panel.ItemInfo) {
  return post<T>({
    url: '/panel/itemIcon/edit',
    data: req,
  })
}

// export function getInfo<T>(id: number) {
//   return post<T>({
//     url: '/aiApplet/getInfo',
//     data: { id },
//   })
// }

export function getListByGroupId<T>(itemIconGroupId: number | undefined) {
  return post<T>({
    url: '/panel/itemIcon/getListByGroupId',
    data: { itemIconGroupId },
  })
}

export function deletes<T>(ids: number[]) {
  return post<T>({
    url: '/panel/itemIcon/deletes',
    data: { ids },
  })
}

export function saveSort<T>(data: Panel.ItemIconSortRequest) {
  return post<T>({
    url: '/panel/itemIcon/saveSort',
    data,
  })
}

export function getSiteFavicon<T>(url: string) {
  return post<T>({
    url: '/panel/itemIcon/getSiteFavicon',
    data: { url },
  })
}

// 获取站点图标候选项（可能多个），供用户挑选
export function getSiteFaviconOptions<T>(url: string) {
  return post<T>({
    url: '/panel/itemIcon/getSiteFaviconOptions',
    data: { url },
  })
}

// 保存用户选中的图标到服务器
export function saveSiteFavicon<T>(url: string, sourceUrl: string) {
  return post<T>({
    url: '/panel/itemIcon/saveSiteFavicon',
    data: { url, sourceUrl },
  })
}
