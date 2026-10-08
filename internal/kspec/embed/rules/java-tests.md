---
paths:
  - "src/test/java/**/*.java"
  - "**/*Test.java"
  - "**/*IT.java"
---

# Testes Java

## Framework

Utilize JUnit 5 para testes. Nunca JUnit 4. Use AssertJ para assertions (nunca `assertEquals` do JUnit).

**Exemplo:**
```java
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.DisplayName;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class UserServiceTest {

  @Test
  @DisplayName("should create user with valid data")
  void shouldCreateUserWithValidData() {
    var service = new UserService(new InMemoryUserRepository());
    var user = service.create(new CreateUserRequest("John", "john@example.com"));
    assertThat(user.name()).isEqualTo("John");
  }
}
```

## Execucao

Para rodar os testes, utilize o comando:
```bash
mvn test
```

## Estrutura AAA/GWT

Siga o principio **Arrange, Act, Assert** ou **Given, When, Then** para garantir o maximo de organizacao e legibilidade dentro dos testes.

**Exemplo:**
```java
@Test
@DisplayName("should calculate total price with discount")
void shouldCalculateTotalPriceWithDiscount() {
  // Arrange
  var items = List.of(
      new OrderItem(100.0, 2),
      new OrderItem(50.0, 1)
  );
  var discountPercentage = 10;

  // Act
  var total = calculator.calculateTotal(items, discountPercentage);

  // Assert
  assertThat(total).isEqualTo(225.0); // (200 + 50) * 0.9
}
```

## Nomenclatura

Use `@DisplayName` com descricoes em ingles: `"should return user when valid id"`. Classes de teste: `NomeClasseTest.java`. Use `@Nested` para agrupar cenarios.

**Exemplo:**
```java
// ❌ Evite
@Test
void test1() {}

@Test
void testUser() {}

// ✅ Prefira
class OrderServiceTest {

  @Nested
  @DisplayName("create")
  class Create {

    @Test
    @DisplayName("should create order with valid items")
    void shouldCreateOrderWithValidItems() {}

    @Test
    @DisplayName("should throw when items list is empty")
    void shouldThrowWhenItemsListIsEmpty() {}
  }

  @Nested
  @DisplayName("cancel")
  class Cancel {

    @Test
    @DisplayName("should cancel pending order")
    void shouldCancelPendingOrder() {}

    @Test
    @DisplayName("should throw when order is already shipped")
    void shouldThrowWhenOrderIsAlreadyShipped() {}
  }
}
```

## Independencia

Nao crie dependencia entre os testes. Deve ser possivel rodar cada um de forma independente. Use `@BeforeEach` para setup, nunca estado compartilhado.

**Exemplo:**
```java
// ❌ Evite
class UserServiceTest {
  private static User sharedUser;

  @Test
  void shouldCreateUser() {
    sharedUser = service.create(new CreateUserRequest("John", "john@example.com"));
  }

  @Test
  void shouldUpdateUser() {
    service.update(sharedUser.id(), new UpdateUserRequest("Jane")); // depende do anterior
  }
}

// ✅ Prefira
class UserServiceTest {
  private UserService service;

  @BeforeEach
  void setUp() {
    service = new UserService(new InMemoryUserRepository());
  }

  @Test
  @DisplayName("should create user")
  void shouldCreateUser() {
    var user = service.create(new CreateUserRequest("John", "john@example.com"));
    assertThat(user.name()).isEqualTo("John");
  }

  @Test
  @DisplayName("should update user")
  void shouldUpdateUser() {
    var user = service.create(new CreateUserRequest("John", "john@example.com"));
    var updated = service.update(user.id(), new UpdateUserRequest("Jane"));
    assertThat(updated.name()).isEqualTo("Jane");
  }
}
```

## Mocks com Mockito

Use `@Mock` e `@InjectMocks` com `@ExtendWith(MockitoExtension.class)`. Use `when().thenReturn()` para stubs. Use `verify()` para verificar interacoes. Nunca `@MockBean` em testes unitarios.

**Exemplo:**
```java
@ExtendWith(MockitoExtension.class)
class OrderServiceTest {

  @Mock
  private OrderRepository orderRepository;

  @Mock
  private PaymentService paymentService;

  @InjectMocks
  private OrderService orderService;

  @Test
  @DisplayName("should create order and process payment")
  void shouldCreateOrderAndProcessPayment() {
    // Arrange
    var request = new CreateOrderRequest(List.of(new OrderItem("product-1", 2)));
    var savedOrder = new Order("order-1", request.items(), OrderStatus.CREATED);
    when(orderRepository.save(any(Order.class))).thenReturn(savedOrder);

    // Act
    var result = orderService.create(request);

    // Assert
    assertThat(result.id()).isEqualTo("order-1");
    assertThat(result.status()).isEqualTo(OrderStatus.CREATED);
    verify(paymentService).process(any(Order.class));
    verify(orderRepository).save(any(Order.class));
  }
}
```

## AssertJ

Prefira AssertJ sobre JUnit assertions. Use fluent assertions encadeadas.

**Exemplo:**
```java
// ❌ Evite
assertEquals("John", user.name());
assertNotNull(user.id());
assertTrue(users.size() > 0);

// ✅ Prefira
assertThat(user.name()).isEqualTo("John");
assertThat(user.id()).isNotNull();
assertThat(users).isNotEmpty().hasSize(3);

// Assertions em colecoes
assertThat(users)
    .hasSize(3)
    .extracting(User::name)
    .containsExactly("Alice", "Bob", "Charlie");

// Assertions em excecoes
assertThatThrownBy(() -> service.findById("invalid"))
    .isInstanceOf(UserNotFoundException.class)
    .hasMessageContaining("invalid");
```

## Testes de Integracao

Use `@SpringBootTest` para testes de integracao. `@WebMvcTest` para testar controllers isoladamente. `MockMvc` para simular requests HTTP.

**Exemplo:**
```java
@WebMvcTest(UserController.class)
class UserControllerTest {

  @Autowired
  private MockMvc mockMvc;

  @MockBean
  private UserService userService;

  @Test
  @DisplayName("should return user by id")
  void shouldReturnUserById() throws Exception {
    // Arrange
    var user = new UserDTO("1", "John", "john@example.com", LocalDateTime.now());
    when(userService.findById("1")).thenReturn(user);

    // Act & Assert
    mockMvc.perform(get("/users/1"))
        .andExpect(status().isOk())
        .andExpect(jsonPath("$.name").value("John"))
        .andExpect(jsonPath("$.email").value("john@example.com"));
  }

  @Test
  @DisplayName("should return 404 when user not found")
  void shouldReturn404WhenUserNotFound() throws Exception {
    when(userService.findById("999"))
        .thenThrow(new UserNotFoundException("999"));

    mockMvc.perform(get("/users/999"))
        .andExpect(status().isNotFound());
  }
}
```

## Testcontainers

Use Testcontainers para testes que dependem de banco de dados ou servicos externos.

**Exemplo:**
```java
@SpringBootTest
@Testcontainers
class UserRepositoryIT {

  @Container
  static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16")
      .withDatabaseName("testdb");

  @DynamicPropertySource
  static void configureProperties(DynamicPropertyRegistry registry) {
    registry.add("spring.datasource.url", postgres::getJdbcUrl);
    registry.add("spring.datasource.username", postgres::getUsername);
    registry.add("spring.datasource.password", postgres::getPassword);
  }

  @Autowired
  private UserRepository userRepository;

  @Test
  @DisplayName("should save and find user by email")
  void shouldSaveAndFindUserByEmail() {
    // Arrange
    var user = new User("John", "john@example.com");
    userRepository.save(user);

    // Act
    var found = userRepository.findByEmail("john@example.com");

    // Assert
    assertThat(found).isPresent();
    assertThat(found.get().getName()).isEqualTo("John");
  }
}
```

## Cobertura

Garanta 100% de cobertura com JaCoCo.

**Exemplo:**
```bash
# Executar testes com cobertura
mvn test jacoco:report

# Relatorio em target/site/jacoco/index.html
```

## Fixtures e Builders

Use metodos factory ou builders para criar objetos de teste. Evite dados hardcoded repetidos.

**Exemplo:**
```java
// ✅ Prefira - factory methods
class UserFixture {

  public static User defaultUser() {
    return new User("John", "john@example.com");
  }

  public static User withName(String name) {
    return new User(name, name.toLowerCase() + "@example.com");
  }

  public static CreateUserRequest createRequest() {
    return new CreateUserRequest("John", "john@example.com");
  }
}

// Uso nos testes
@Test
void shouldCreateUser() {
  var user = service.create(UserFixture.createRequest());
  assertThat(user.name()).isEqualTo("John");
}
```
