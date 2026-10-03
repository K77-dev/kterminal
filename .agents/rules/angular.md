---
paths:
  - "frontend/src/**/*.component.ts"
  - "frontend/src/**/*.html"
  - "frontend/src/**/*.scss"
---

# Angular

## Standalone Components

Use standalone components. Nunca NgModules para componentes novos. `standalone: true` em todos os `@Component`. Importe dependencias diretamente no componente.

**Exemplo:**
```typescript
// ❌ Evite - NgModule
@NgModule({
  declarations: [UserCardComponent],
  imports: [CommonModule],
  exports: [UserCardComponent]
})
export class UserModule {}

// ✅ Prefira - standalone
@Component({
  selector: 'app-user-card',
  standalone: true,
  imports: [DatePipe],
  templateUrl: './user-card.component.html',
  styleUrl: './user-card.component.scss'
})
export class UserCardComponent {
  @Input({ required: true }) user!: User;
}
```

## Signals

Use Signals para estado reativo (`signal()`, `computed()`, `effect()`). Prefira signals sobre BehaviorSubject para estado local do componente.

**Exemplo:**
```typescript
// ❌ Evite - BehaviorSubject para estado local
@Component({ ... })
export class CounterComponent {
  private count$ = new BehaviorSubject(0);
  doubled$ = this.count$.pipe(map(c => c * 2));

  increment() {
    this.count$.next(this.count$.value + 1);
  }
}

// ✅ Prefira - Signals
@Component({ ... })
export class CounterComponent {
  count = signal(0);
  doubled = computed(() => this.count() * 2);

  increment() {
    this.count.update(c => c + 1);
  }
}
```

## Tipagem

TypeScript strict. Interfaces para modelos. Nunca `any`.

**Exemplo:**
```typescript
// ❌ Evite
function getUser(data: any): any {
  return data.user;
}

// ✅ Prefira
interface User {
  id: string;
  name: string;
  email: string;
}

interface ApiResponse<T> {
  data: T;
  message: string;
}
```

## Formularios Tipados

Use Typed Reactive Forms. Nunca template-driven forms para formularios complexos. Use `NonNullableFormBuilder`.

**Exemplo:**
```typescript
// ❌ Evite - template-driven para formularios complexos
// <input [(ngModel)]="user.name">

// ❌ Evite - untyped
this.form = this.fb.group({
  name: [''],
  email: ['']
});

// ✅ Prefira - typed reactive forms
@Component({ ... })
export class UserFormComponent {
  private fb = inject(NonNullableFormBuilder);

  form = this.fb.group({
    name: ['', [Validators.required, Validators.minLength(2)]],
    email: ['', [Validators.required, Validators.email]],
    age: [0, [Validators.required, Validators.min(18)]]
  });

  onSubmit() {
    if (this.form.valid) {
      const value = this.form.getRawValue();
      // value e tipado: { name: string; email: string; age: number }
    }
  }
}
```

## Injecao de Dependencia

Use `inject()` function. Nunca injecao via construtor em componentes novos. `providedIn: 'root'` para services singleton.

**Exemplo:**
```typescript
// ❌ Evite
@Component({ ... })
export class UserListComponent {
  constructor(
    private userService: UserService,
    private router: Router
  ) {}
}

// ✅ Prefira
@Component({ ... })
export class UserListComponent {
  private userService = inject(UserService);
  private router = inject(Router);
}

// Service
@Injectable({ providedIn: 'root' })
export class UserService {
  private http = inject(HttpClient);

  getUsers(): Observable<User[]> {
    return this.http.get<User[]>('/api/users');
  }
}
```

## RxJS

Use RxJS para fluxos assincronos e comunicacao com APIs. Sempre faca unsubscribe com `takeUntilDestroyed()` ou `async` pipe. Evite `subscribe()` aninhado.

**Exemplo:**
```typescript
// ❌ Evite - subscribe aninhado, sem unsubscribe
ngOnInit() {
  this.userService.getUser(this.id).subscribe(user => {
    this.orderService.getOrders(user.id).subscribe(orders => {
      this.orders = orders;
    });
  });
}

// ✅ Prefira - pipe operators + takeUntilDestroyed
export class UserOrdersComponent {
  private destroyRef = inject(DestroyRef);
  private userService = inject(UserService);
  private orderService = inject(OrderService);

  orders = signal<Order[]>([]);

  ngOnInit() {
    this.userService.getUser(this.id).pipe(
      switchMap(user => this.orderService.getOrders(user.id)),
      takeUntilDestroyed(this.destroyRef)
    ).subscribe(orders => this.orders.set(orders));
  }
}

// ✅ Ou com async pipe no template
@Component({
  template: `
    @if (users$ | async; as users) {
      @for (user of users; track user.id) {
        <app-user-card [user]="user" />
      }
    }
  `
})
export class UserListComponent {
  users$ = inject(UserService).getUsers();
}
```

## HTTP Client

Use `HttpClient` com tipagem generica. Centralize chamadas em services. Use functional interceptors (`HttpInterceptorFn`).

**Exemplo:**
```typescript
// Service
@Injectable({ providedIn: 'root' })
export class UserService {
  private http = inject(HttpClient);
  private apiUrl = '/api/users';

  getUsers(): Observable<User[]> {
    return this.http.get<User[]>(this.apiUrl);
  }

  createUser(request: CreateUserRequest): Observable<User> {
    return this.http.post<User>(this.apiUrl, request);
  }
}

// Functional interceptor
export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const authService = inject(AuthService);
  const token = authService.getToken();

  if (token) {
    const cloned = req.clone({
      setHeaders: { Authorization: `Bearer ${token}` }
    });
    return next(cloned);
  }

  return next(req);
};

// Configuracao
export const appConfig: ApplicationConfig = {
  providers: [
    provideHttpClient(withInterceptors([authInterceptor]))
  ]
};
```

## Roteamento

Lazy loading com `loadComponent`. Use functional guards (`CanActivateFn`). Route resolvers para pre-carregamento de dados.

**Exemplo:**
```typescript
// ✅ Prefira - lazy loading com loadComponent
export const routes: Routes = [
  {
    path: 'users',
    loadComponent: () => import('./user-list.component').then(m => m.UserListComponent),
    canActivate: [authGuard]
  },
  {
    path: 'users/:id',
    loadComponent: () => import('./user-detail.component').then(m => m.UserDetailComponent),
    resolve: { user: userResolver }
  }
];

// Functional guard
export const authGuard: CanActivateFn = () => {
  const authService = inject(AuthService);
  const router = inject(Router);

  if (authService.isAuthenticated()) {
    return true;
  }

  return router.createUrlTree(['/login']);
};

// Resolver
export const userResolver: ResolveFn<User> = (route) => {
  const userService = inject(UserService);
  return userService.getUser(route.paramMap.get('id')!);
};
```

## Estrutura de Componentes

Organize por feature. Componentes pequenos (max 300 linhas). Smart components (containers) vs Dumb components (presentational). Input/Output para comunicacao pai-filho.

**Exemplo:**
```typescript
// Smart component (container) - busca dados e gerencia estado
@Component({
  selector: 'app-user-page',
  standalone: true,
  imports: [UserListComponent, UserFilterComponent],
  template: `
    <app-user-filter (filterChange)="onFilterChange($event)" />
    <app-user-list [users]="filteredUsers()" />
  `
})
export class UserPageComponent {
  private userService = inject(UserService);
  users = signal<User[]>([]);
  filter = signal('');
  filteredUsers = computed(() =>
    this.users().filter(u => u.name.includes(this.filter()))
  );

  onFilterChange(filter: string) {
    this.filter.set(filter);
  }
}

// Dumb component (presentational) - so recebe e exibe dados
@Component({
  selector: 'app-user-list',
  standalone: true,
  template: `
    @for (user of users; track user.id) {
      <div class="user-card">{{ user.name }}</div>
    }
  `
})
export class UserListComponent {
  @Input({ required: true }) users!: User[];
}
```

## Estilizacao

SCSS por componente com ViewEncapsulation padrao. Use variaveis CSS ou design tokens.

**Exemplo:**
```scss
// user-card.component.scss
:host {
  display: block;
}

.user-card {
  padding: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);

  &__name {
    font-weight: var(--font-weight-bold);
    color: var(--color-text-primary);
  }

  &__email {
    color: var(--color-text-secondary);
  }
}
```

## Control Flow

Use `@if`, `@for`, `@switch` (nova sintaxe de template). Nunca `*ngIf`, `*ngFor`. `@defer` para lazy rendering. `track` obrigatorio em `@for`.

**Exemplo:**
```html
<!-- ❌ Evite -->
<div *ngIf="user">{{ user.name }}</div>
<div *ngFor="let item of items">{{ item.name }}</div>

<!-- ✅ Prefira -->
@if (user) {
  <div>{{ user.name }}</div>
}

@for (item of items; track item.id) {
  <div>{{ item.name }}</div>
} @empty {
  <p>Nenhum item encontrado.</p>
}

@switch (status) {
  @case ('active') { <span class="badge-active">Ativo</span> }
  @case ('inactive') { <span class="badge-inactive">Inativo</span> }
  @default { <span class="badge-unknown">Desconhecido</span> }
}

<!-- Lazy rendering -->
@defer (on viewport) {
  <app-heavy-chart [data]="chartData" />
} @placeholder {
  <div class="skeleton">Carregando grafico...</div>
}
```

## Testes

Testes com Jasmine e `TestBed`. `ComponentFixture` para testes de componente.

**Exemplo:**
```typescript
import { ComponentFixture, TestBed } from '@angular/core/testing';

describe('UserCardComponent', () => {
  let component: UserCardComponent;
  let fixture: ComponentFixture<UserCardComponent>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [UserCardComponent]
    }).compileComponents();

    fixture = TestBed.createComponent(UserCardComponent);
    component = fixture.componentInstance;
  });

  it('should render user name', () => {
    component.user = { id: '1', name: 'John', email: 'john@example.com' };
    fixture.detectChanges();

    const element = fixture.nativeElement.querySelector('.user-card__name');
    expect(element.textContent).toContain('John');
  });

  it('should render user email', () => {
    component.user = { id: '1', name: 'John', email: 'john@example.com' };
    fixture.detectChanges();

    const element = fixture.nativeElement.querySelector('.user-card__email');
    expect(element.textContent).toContain('john@example.com');
  });
});

// Testes de service com HttpClientTestingModule
describe('UserService', () => {
  let service: UserService;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()]
    });
    service = TestBed.inject(UserService);
    httpMock = TestBed.inject(HttpTestingController);
  });

  it('should fetch users', () => {
    const mockUsers: User[] = [{ id: '1', name: 'John', email: 'john@example.com' }];

    service.getUsers().subscribe(users => {
      expect(users).toEqual(mockUsers);
    });

    const req = httpMock.expectOne('/api/users');
    expect(req.request.method).toBe('GET');
    req.flush(mockUsers);
  });
});
```
