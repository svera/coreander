const createMenuItemRadioGroup = (label, arr, onclick) => {
    const group = document.createElement('ul')
    group.setAttribute('role', 'group')
    group.setAttribute('aria-label', label)
    const map = new Map()
    const select = value => {
        onclick(value)
        const item = map.get(value)
        for (const child of group.children)
            child.setAttribute('aria-checked', child === item ? 'true' : 'false')
    }
    for (const [label, value] of arr) {
        const item = document.createElement('li')
        item.setAttribute('role', 'menuitemradio')
        item.innerText = label
        item.onclick = () => select(value)
        map.set(value, item)
        group.append(item)
    }
    return { element: group, select }
}

const createMenuItemCheckbox = (label, onclick) => {
    const item = document.createElement('li')
    item.setAttribute('role', 'menuitemcheckbox')
    item.setAttribute('aria-checked', 'false')
    item.innerText = label
    let checked = false
    const toggle = () => {
        checked = !checked
        item.setAttribute('aria-checked', checked ? 'true' : 'false')
        onclick(checked)
    }
    const setChecked = value => {
        checked = value
        item.setAttribute('aria-checked', checked ? 'true' : 'false')
    }
    item.onclick = toggle
    return { element: item, setChecked }
}

export const createMenu = arr => {
    const groups = {}
    const element = document.createElement('ul')
    element.setAttribute('role', 'menu')
    const hide = () => element.classList.remove('show')
    for (const { name, label, type, items, onclick, content } of arr) {
        if (type === 'separator') {
            const separator = document.createElement('hr')
            separator.setAttribute('role', 'separator')
            element.append(separator)
        } else if (type === 'checkbox') {
            const widget = createMenuItemCheckbox(label, onclick)
            if (name) groups[name] = widget
            element.append(widget.element)
        } else if (type === 'radio') {
            const widget = createMenuItemRadioGroup(label, items, onclick)
            if (name) groups[name] = widget
            element.append(widget.element)
        } else if (type === 'custom') {
            const item = document.createElement('li')
            item.classList.add('menu-custom-item')
            if (label) {
                const labelSpan = document.createElement('span')
                labelSpan.classList.add('menu-item-label')
                labelSpan.innerText = label
                item.append(labelSpan)
            }
            if (content) {
                item.append(content)
            }
            element.append(item)
            if (name) groups[name] = { element: item }
        }
    }
    // TODO: keyboard events
    window.addEventListener('blur', () => hide())
    window.addEventListener('click', e => {
        if (!element.parentNode.contains(e.target)) hide()
    })
    return { element, groups }
}
