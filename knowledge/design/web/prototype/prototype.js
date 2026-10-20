(function () {
  class PageLogic {
    constructor(props) {
      this.props = props || {};
      this.state = {};
    }
    setState(patch) {
      Object.assign(this.state, typeof patch === 'function' ? patch(this.state) : patch);
      render();
    }
  }

  let component;
  let template;
  let host;
  const savedValues = {};

  function lookup(vals, path) {
    path = path.trim();
    if (path === 'true') return true;
    if (path === 'false') return false;
    return path.split('.').reduce((o, k) => (o == null ? o : o[k]), vals);
  }

  function interpolate(text, vals) {
    return text.replace(/\{\{([^}]+)\}\}/g, (_, p) => {
      const v = lookup(vals, p);
      return v == null ? '' : String(v);
    });
  }

  function process(node, vals) {
    Array.from(node.childNodes).forEach((child) => {
      if (child.nodeType === 3) {
        if (child.nodeValue.includes('{{')) child.nodeValue = interpolate(child.nodeValue, vals);
        return;
      }
      if (child.nodeType !== 1) return;
      if (child.tagName === 'X-IF') {
        const cond = lookup(vals, child.getAttribute('value').replace(/[{}]/g, ''));
        if (cond) {
          process(child, vals);
          child.replaceWith(...Array.from(child.childNodes));
        } else {
          child.remove();
        }
        return;
      }
      Array.from(child.attributes).forEach((attr) => {
        if (!attr.value.includes('{{')) return;
        const whole = attr.value.match(/^\s*\{\{([^}]+)\}\}\s*$/);
        if (attr.name.startsWith('on') && whole) {
          child.removeAttribute(attr.name);
          const fn = lookup(vals, whole[1]);
          if (typeof fn === 'function') child.addEventListener(attr.name.slice(2), fn);
        } else {
          child.setAttribute(attr.name, interpolate(attr.value, vals));
        }
      });
      process(child, vals);
    });
  }

  function focusKey(element) {
    if (!element || !host.contains(element)) return null;
    if (element.id) return { id: element.id };
    const path = [];
    let node = element;
    while (node && node !== host) {
      path.unshift(Array.prototype.indexOf.call(node.parentNode.children, node));
      node = node.parentNode;
    }
    return { path };
  }

  function restoreFocus(key) {
    if (!key) return;
    let target = null;
    if (key.id) {
      target = document.getElementById(key.id);
    } else {
      target = host;
      for (const index of key.path) {
        target = target && target.children[index];
      }
    }
    if (!target || !target.isConnected || target === host || target.offsetParent === null) {
      target = host.querySelector('h1');
      if (target) target.setAttribute('tabindex', '-1');
    }
    if (target && typeof target.focus === 'function') target.focus();
  }

  function captureValues() {
    const values = {};
    host.querySelectorAll('input[id], textarea[id]').forEach((field) => {
      values[field.id] = field.type === 'radio' || field.type === 'checkbox' ? field.checked : field.value;
    });
    return values;
  }

  function restoreValues(values) {
    Object.keys(values).forEach((id) => {
      const field = document.getElementById(id);
      if (!field) return;
      if (field.type === 'radio' || field.type === 'checkbox') field.checked = values[id]; else field.value = values[id];
    });
  }

  function render() {
    const key = focusKey(document.activeElement);
    const values = Object.assign(savedValues, captureValues());
    const fragment = template.content.cloneNode(true);
    process(fragment, component ? component.renderVals() : {});
    host.replaceChildren(fragment);
    restoreValues(values);
    restoreFocus(key);
  }

  const icons = {
    today: '<circle cx="8" cy="8" r="2.75"/><path d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M3.4 12.6l1-1M11.6 4.4l1-1"/>',
    calendar: '<rect x="2" y="3" width="12" height="11" rx="1.5"/><path d="M2 6.5h12M5.5 1.5v3M10.5 1.5v3"/>',
    people: '<circle cx="6" cy="5.5" r="2.5"/><path d="M1.75 13.5c.5-2.4 2.2-3.75 4.25-3.75s3.75 1.35 4.25 3.75"/><path d="M10.5 3.2a2.4 2.4 0 0 1 0 4.6M12 9.9c1.2.5 2 1.7 2.25 3.6"/>',
    group: '<circle cx="8" cy="4.5" r="2"/><circle cx="3.5" cy="9" r="1.75"/><circle cx="12.5" cy="9" r="1.75"/><path d="M5 14c.4-1.7 1.5-2.6 3-2.6s2.6.9 3 2.6"/>',
    tasks: '<rect x="3" y="2" width="10" height="12.5" rx="1.5"/><path d="M6 2V1.5h4V2M5.5 7l1.25 1.25L9 6M5.5 11h5"/>',
    chat: '<path d="M2 3.5A1.5 1.5 0 0 1 3.5 2h9A1.5 1.5 0 0 1 14 3.5v6a1.5 1.5 0 0 1-1.5 1.5H7l-3.5 3v-3A1.5 1.5 0 0 1 2 9.5z"/>',
    home: '<path d="M2 7.25 8 2l6 5.25V14H2z"/><path d="M6.25 14V10h3.5v4"/>',
    tutors: '<circle cx="8" cy="5" r="2.75"/><path d="M2.75 14c.6-2.9 2.6-4.5 5.25-4.5s4.65 1.6 5.25 4.5"/>',
    bell: '<path d="M4 11V7a4 4 0 0 1 8 0v4l1.25 1.5H2.75z"/><path d="M6.5 14a1.5 1.5 0 0 0 3 0"/>',
    settings: '<circle cx="8" cy="8" r="2"/><path d="M8 1.75v1.5M8 12.75v1.5M1.75 8h1.5M12.75 8h1.5M3.6 3.6l1.05 1.05M11.35 11.35l1.05 1.05M3.6 12.4l1.05-1.05M11.35 4.65l1.05-1.05"/>',
    collapse: '<path d="M9.5 4 5.5 8l4 4M13 4 9 8l4 4"/>',
    expand: '<path d="M6.5 4l4 4-4 4M3 4l4 4-4 4"/>',
    menu: '<path d="M2.5 4h11M2.5 8h11M2.5 12h11"/>'
  };

  function svg(name) {
    return '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + icons[name] + '</svg>';
  }

  const roles = {
    tutor: {
      name: 'Анна Смирнова',
      initial: 'А',
      notifications: 4,
      items: [
        ['today', 'today', 'Сегодня', 'today.html'],
        ['schedule', 'calendar', 'Расписание', null],
        ['students', 'people', 'Ученики', null],
        ['groups', 'group', 'Группы', null],
        ['homework', 'tasks', 'Задания', 'builder.html', 3],
        ['chat', 'chat', 'Сообщения', null, 2]
      ]
    },
    student: {
      name: 'Борис Петров',
      initial: 'Б',
      notifications: 1,
      items: [
        ['home', 'home', 'Главная', 'student-home.html'],
        ['schedule', 'calendar', 'Расписание', null],
        ['homework', 'tasks', 'Задания', null, 2],
        ['chat', 'chat', 'Сообщения', null, 2],
        ['tutors', 'tutors', 'Мои репетиторы', null]
      ]
    }
  };

  const narrowQuery = window.matchMedia('(max-width: 1023px)');

  function readHidden() {
    try { return localStorage.getItem('ovc-sidebar-hidden') === '1'; } catch (e) { return false; }
  }

  function saveHidden(value) {
    try { localStorage.setItem('ovc-sidebar-hidden', value ? '1' : '0'); } catch (e) { return; }
  }

  function countLabel(count) {
    return count ? '<span class="side-count">' + count + '</span>' : '';
  }

  function buildSidebar(role, roleKey, pageKey) {
    const side = document.createElement('nav');
    side.className = 'side';
    side.id = 'sidebar';
    side.setAttribute('aria-label', 'Разделы');

    let html = '<div class="side-head">'
      + '<button type="button" class="account" data-account aria-expanded="false" aria-controls="account-menu"><span class="avatar" aria-hidden="true">' + role.initial + '</span><span class="side-name">' + role.name + '</span></button>'
      + '<button type="button" class="icon-btn" data-hide aria-label="Скрыть боковую панель">' + svg('collapse') + '</button></div>'
      + '<div id="account-menu" hidden style="padding: 4px 0 0 36px">'
      + '<span class="side-item stub" title="Этого экрана пока нет в прототипе">Настройки</span>'
      + '<span class="side-item stub" title="Этого экрана пока нет в прототипе">Выйти</span></div>'
      + '<div style="height: 10px"></div>';
    role.items.forEach(([key, icon, title, href, count]) => {
      const on = key === pageKey;
      const countHtml = countLabel(count);
      if (href) {
        html += '<a class="side-item' + (on ? ' on' : '') + '" href="' + href + '"' + (on ? ' aria-current="page"' : '') + '>' + svg(icon) + title + countHtml + '</a>';
      } else {
        html += '<span class="side-item stub" title="Этого экрана пока нет в прототипе">' + svg(icon) + title + countHtml + '</span>';
      }
    });
    html += '<span class="side-item stub" style="margin-top: 8px" title="Этого экрана пока нет в прототипе">' + svg('bell') + 'Уведомления' + countLabel(role.notifications) + '</span>';
    html += '<span class="side-item stub" title="Этого экрана пока нет в прототипе">' + svg('settings') + 'Настройки</span>';
    html += '<div class="side-foot"><div class="small muted">Прототип: смотреть как</div><div class="switch">'
      + '<a href="today.html" class="' + (roleKey === 'tutor' ? 'on' : '') + '"' + (roleKey === 'tutor' ? ' aria-current="page"' : '') + '>Репетитор</a>'
      + '<a href="student-home.html" class="' + (roleKey === 'student' ? 'on' : '') + '"' + (roleKey === 'student' ? ' aria-current="page"' : '') + '>Ученик</a></div></div>';
    side.innerHTML = html;

    const account = side.querySelector('[data-account]');
    const menu = side.querySelector('#account-menu');
    account.addEventListener('click', () => {
      const open = menu.hidden;
      menu.hidden = !open;
      account.setAttribute('aria-expanded', String(open));
    });
    return side;
  }

  function buildShell(content) {
    const roleKey = document.body.dataset.role || 'tutor';
    const pageKey = document.body.dataset.page || '';
    const role = roles[roleKey];
    const hasNews = role.notifications > 0 || role.items.some((item) => item[4]);
    const shell = document.createElement('div');
    shell.className = 'shell';
    const side = buildSidebar(role, roleKey, pageKey);
    const dot = hasNews ? '<span class="badge-dot" aria-hidden="true"></span>' : '';

    const reopen = document.createElement('button');
    reopen.type = 'button';
    reopen.className = 'icon-btn reopen';
    reopen.setAttribute('aria-label', 'Показать боковую панель' + (hasNews ? ', есть новое' : ''));
    reopen.setAttribute('aria-controls', 'sidebar');
    reopen.innerHTML = svg('expand') + dot;

    const bar = document.createElement('div');
    bar.className = 'mobile-bar';
    bar.innerHTML = '<button type="button" class="icon-btn" data-open aria-controls="sidebar" aria-label="Показать боковую панель' + (hasNews ? ', есть новое' : '') + '">' + svg('menu') + dot + '</button>'
      + '<span class="mobile-title">' + document.title.split(' · ')[0] + '</span>'
      + '<button type="button" class="icon-btn" aria-label="Уведомления: ' + role.notifications + ' непрочитанных">' + svg('bell') + '</button>'
      + (role.notifications ? '<span class="bar-count" aria-hidden="true">' + role.notifications + '</span>' : '');
    const openButton = bar.querySelector('[data-open]');

    const scrim = document.createElement('div');
    scrim.className = 'scrim';

    let overlayOpen = false;

    function setOverlay(open) {
      overlayOpen = open;
      side.classList.toggle('closed', !open);
      scrim.classList.toggle('show', open);
      if (open) {
        side.setAttribute('role', 'dialog');
        side.setAttribute('aria-modal', 'true');
        (side.querySelector('.side-item.on') || side.querySelector('button, a')).focus();
      } else {
        side.removeAttribute('role');
        side.removeAttribute('aria-modal');
        openButton.focus();
      }
    }

    function setHidden(value) {
      side.classList.toggle('closed', value);
      reopen.classList.toggle('show', value);
      saveHidden(value);
      (value ? reopen : side.querySelector('[data-hide]')).focus();
    }

    function applyLayout() {
      if (narrowQuery.matches) {
        overlayOpen = false;
        side.classList.add('closed');
        scrim.classList.remove('show');
        reopen.classList.remove('show');
      } else {
        side.removeAttribute('role');
        side.removeAttribute('aria-modal');
        scrim.classList.remove('show');
        const hidden = readHidden();
        side.classList.toggle('closed', hidden);
        reopen.classList.toggle('show', hidden);
      }
    }

    side.querySelector('[data-hide]').addEventListener('click', () => {
      if (narrowQuery.matches) setOverlay(false); else setHidden(true);
    });
    reopen.addEventListener('click', () => setHidden(false));
    openButton.addEventListener('click', () => setOverlay(true));
    scrim.addEventListener('click', () => setOverlay(false));
    side.addEventListener('keydown', (event) => {
      if (!overlayOpen) return;
      if (event.key === 'Escape') {
        setOverlay(false);
        return;
      }
      if (event.key !== 'Tab') return;
      const focusable = Array.from(side.querySelectorAll('a, button')).filter((el) => el.offsetParent !== null);
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    });
    narrowQuery.addEventListener('change', applyLayout);
    applyLayout();

    const main = document.createElement('div');
    main.className = 'content';
    main.append(bar, content);
    shell.append(side, scrim, main, reopen);
    return shell;
  }

  document.addEventListener('DOMContentLoaded', () => {
    const source = document.querySelector('x-page');
    template = document.createElement('template');
    template.innerHTML = source.innerHTML;
    const script = document.querySelector('script[type="text/x-page"]');
    if (script) {
      const Component = new Function('PageLogic', script.textContent + '\nreturn Component;')(PageLogic);
      component = new Component({});
    }
    host = document.createElement('div');
    host.style.display = 'contents';
    source.replaceWith(buildShell(host));
    render();
  });
})();
