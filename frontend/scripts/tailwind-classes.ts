import ts from 'typescript';

export interface ClassUse {
  readonly token: string;
  readonly file: string;
}

function isClassNameAttribute(node: ts.Node): node is ts.JsxAttribute {
  return ts.isJsxAttribute(node) && ts.isIdentifier(node.name) && node.name.text === 'className';
}

function wholeTokens(text: string, openStart: boolean, openEnd: boolean): string[] {
  const tokens = text.split(/\s+/).filter((token) => token !== '');
  const startsCut = openStart && !/^\s/.test(text);
  const endsCut = openEnd && !/\s$/.test(text);
  return tokens.filter((_token, index) => !(startsCut && index === 0) && !(endsCut && index === tokens.length - 1));
}

function templateTokens(template: ts.TemplateExpression): string[] {
  const tokens = wholeTokens(template.head.text, false, true);
  template.templateSpans.forEach((span, index) => {
    const last = index === template.templateSpans.length - 1;
    tokens.push(...wholeTokens(span.literal.text, true, !last));
  });
  return tokens;
}

function collect(node: ts.Node, checker: ts.TypeChecker, visited: Set<ts.Node>, tokens: string[]): void {
  if (visited.has(node)) {
    return;
  }
  visited.add(node);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    tokens.push(...wholeTokens(node.text, false, false));
    return;
  }
  if (ts.isTemplateExpression(node)) {
    tokens.push(...templateTokens(node));
    for (const span of node.templateSpans) {
      collect(span.expression, checker, visited, tokens);
    }
    return;
  }
  if (ts.isCallExpression(node)) {
    return;
  }
  if (ts.isIdentifier(node)) {
    const found = checker.getSymbolAtLocation(node);
    const symbol =
      found !== undefined && (found.flags & ts.SymbolFlags.Alias) !== 0 ? checker.getAliasedSymbol(found) : found;
    for (const declaration of symbol?.declarations ?? []) {
      if (ts.isVariableDeclaration(declaration) && declaration.initializer !== undefined) {
        collect(declaration.initializer, checker, visited, tokens);
      }
    }
    return;
  }
  if (ts.isElementAccessExpression(node)) {
    collect(node.expression, checker, visited, tokens);
    return;
  }
  if (ts.isShorthandPropertyAssignment(node)) {
    for (const declaration of checker.getShorthandAssignmentValueSymbol(node)?.declarations ?? []) {
      if (ts.isVariableDeclaration(declaration) && declaration.initializer !== undefined) {
        collect(declaration.initializer, checker, visited, tokens);
      }
    }
    return;
  }
  if (ts.isPropertyAssignment(node)) {
    collect(node.initializer, checker, visited, tokens);
    return;
  }
  ts.forEachChild(node, (child) => {
    collect(child, checker, visited, tokens);
  });
}

export function classUses(program: ts.Program, files: readonly string[]): ClassUse[] {
  const checker = program.getTypeChecker();
  const uses: ClassUse[] = [];
  for (const file of files) {
    const source = program.getSourceFile(file);
    if (source === undefined) {
      throw new Error(`${file} is not part of the program`);
    }
    const visit = (node: ts.Node): void => {
      if (isClassNameAttribute(node) && node.initializer !== undefined) {
        const tokens: string[] = [];
        collect(node.initializer, checker, new Set(), tokens);
        uses.push(...tokens.map((token) => ({ token, file })));
        return;
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  return uses;
}

function escapedSelector(token: string): string {
  const escaped = token.replace(/[^A-Za-z0-9_-]/g, (character) => `\\${character}`);
  return escaped.replace(/^(-?)([0-9])/, (_match, dash: string, digit: string) => `${dash}\\3${digit} `);
}

export function hasSelector(token: string, css: string): boolean {
  const selector = `.${escapedSelector(token)}`;
  let from = css.indexOf(selector);
  while (from !== -1) {
    const next = css.charAt(from + selector.length);
    if (!/[A-Za-z0-9_\\-]/.test(next)) {
      return true;
    }
    from = css.indexOf(selector, from + 1);
  }
  return false;
}

export function missingClasses(uses: readonly ClassUse[], css: string): ClassUse[] {
  const seen = new Set<string>();
  return uses.filter((use) => {
    const key = `${use.file}\u0000${use.token}`;
    if (seen.has(key) || hasSelector(use.token, css)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}
