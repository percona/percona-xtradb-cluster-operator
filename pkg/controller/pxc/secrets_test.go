package pxc

import (
	"context"
	"crypto/rand"
	"io"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	api "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	v1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/percona/percona-xtradb-cluster-operator/pkg/pxc/users"
)

var _ = Describe("Secrets generation", Ordered, func() {
	ctx := context.Background()
	const ns = "sec-gen"
	const defaultSymbols = "!#$%&()*+,-.<=>?@[]^_{}~"
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: ns,
		},
	}
	crName := ns + "-cr"
	crNamespacedName := types.NamespacedName{Name: crName, Namespace: ns}
	cr, err := readDefaultCR(crName, ns)
	It("Should read default cr.yaml", func() {
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeAll(func() {
		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))
	})

	AfterAll(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	Context("Create cluster with default password generation behavior", func() {
		It("Should create PerconaXtraDBCluster", func() {
			Expect(k8sClient.Create(ctx, cr)).Should(Succeed())
		})

		It("Should reconcile PerconaXtraDBCluster", func() {
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: crNamespacedName,
			})
			Expect(err).To(Succeed())
		})
		userSecrets := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-secrets",
				Namespace: cr.Namespace,
			},
		}
		It("Should generate user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecrets), userSecrets)).To(Succeed())
		})
		for password := range userSecrets.Data {
			It("Should generate passwords with default symbols", func() {
				Expect(strings.ContainsAny(password, defaultSymbols)).To(BeTrue())
			})
			It("Should generate passwords with default length constraints", func() {
				Expect(len(password)).To(BeNumerically(">=", 16))
				Expect(len(password)).To(BeNumerically("<=", 20))
			})
		}
	})

	Context("Check user secrets generation with custom password generation options", func() {
		cr := &v1.PerconaXtraDBCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: ns,
			},
		}
		userSecrets := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "-secrets",
				Namespace: cr.Namespace,
			},
		}
		It("Should retrieve cluster configuration", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cr), cr)).Should(Succeed())
		})
		It("Should retrieve current user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecrets), userSecrets)).To(Succeed())
		})
		It("Should update PerconaXtraDBCluster with custom options", func() {
			cr.Spec.PasswordGenerationOptions = &v1.PasswordGenerationOptions{
				Symbols:   "",
				MinLength: 22,
				MaxLength: 30,
			}
			Expect(k8sClient.Update(ctx, cr)).Should(Succeed())
		})
		It("Should reconcile PerconaXtraDBCluster", func() {
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: crNamespacedName,
			})
			Expect(err).To(Succeed())
		})
		It("Should not change existing user secrets", func() {
			userSecretsBis := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cr.Name + "-secrets",
					Namespace: cr.Namespace,
				},
			}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecretsBis), userSecretsBis)).To(Succeed())
			Expect(userSecretsBis.Data).Should(Equal(userSecrets.Data))
		})
		It("Should remove existing user secret", func() {
			Expect(k8sClient.Delete(ctx, userSecrets)).Should(Succeed())
		})
		It("Should reconcile PerconaXtraDBCluster", func() {
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: crNamespacedName,
			})
			Expect(err).To(Succeed())
		})
		It("Should generate user secrets", func() {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(userSecrets), userSecrets)).To(Succeed())
		})
		for password := range userSecrets.Data {
			It("Should generate passwords without symbols", func() {
				Expect(strings.ContainsAny(password, defaultSymbols)).To(BeFalse())
			})
			It("Should generate passwords with length constraints", func() {
				Expect(len(password)).To(BeNumerically(">=", cr.Spec.PasswordGenerationOptions.MinLength))
				Expect(len(password)).To(BeNumerically("<=", cr.Spec.PasswordGenerationOptions.MaxLength))
			})
		}
	})
})

type repeatingReader struct {
	pattern []byte
	pos     int
	reads   int
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	if len(r.pattern) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	for i := range p {
		p[i] = r.pattern[r.pos]
		r.pos = (r.pos + 1) % len(r.pattern)
	}
	r.reads++
	if r.reads > 10000 {
		panic("too many reads: likely stuck in crypto/rand.Int retry loop. Try using a different pattern that produces values < max")
	}
	return len(p), nil
}

func TestGeneratePass(t *testing.T) {
	t.Run("password length constraints", func(t *testing.T) {
		tests := map[string]struct {
			minLength int
			maxLength int
		}{
			"fixed length 16": {16, 16},
			"fixed length 32": {32, 32},
			"range 10-20":     {10, 20},
			"range 16-32":     {16, 32},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				secretOptions := &api.PasswordGenerationOptions{
					Symbols:   "",
					MinLength: tt.minLength,
					MaxLength: tt.maxLength,
				}

				for i := 0; i < 100; i++ {
					p, err := generatePass("", secretOptions)
					require.NoError(t, err)
					assert.GreaterOrEqual(t, len(p), tt.minLength, "password length should be >= minLength")
					assert.LessOrEqual(t, len(p), tt.maxLength, "password length should be <= maxLength")
				}
			})
		}
	})

	t.Run("password character pool", func(t *testing.T) {
		tests := map[string]struct {
			symbols         string
			expectedPool    string
			unexpectedChars string
		}{
			"no symbols": {
				symbols:         "",
				expectedPool:    passBaseSymbols,
				unexpectedChars: "!@#$%^&*",
			},
			"with symbols": {
				symbols:         "!@#$",
				expectedPool:    passBaseSymbols + "!@#$",
				unexpectedChars: "%^&*",
			},
			"all symbols": {
				symbols:         "!#$%&()*+,-.<=>?@[]^_{}~",
				expectedPool:    passBaseSymbols + "!#$%&()*+,-.<=>?@[]^_{}~",
				unexpectedChars: "|\\;:",
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				secretOptions := &api.PasswordGenerationOptions{
					Symbols:   tt.symbols,
					MinLength: 20,
					MaxLength: 20,
				}

				for i := 0; i < 100; i++ {
					p, err := generatePass("", secretOptions)
					require.NoError(t, err)

					for _, char := range string(p) {
						assert.Contains(t, tt.expectedPool, string(char),
							"password contains unexpected character: %c", char)
					}

					for _, char := range tt.unexpectedChars {
						if !strings.Contains(tt.expectedPool, string(char)) {
							assert.NotContains(t, string(p), string(char),
								"password should not contain: %c", char)
						}
					}
				}
			})
		}
	})

	t.Run("symbol guarantee", func(t *testing.T) {
		tests := map[string]struct {
			symbols string
		}{
			"single symbol":    {"%"},
			"two symbols":      {"*&"},
			"multiple symbols": {"!@#$%"},
			"many symbols":     {"!#$%&()*+,-.<=>?@[]^_{}~"},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				secretOptions := &api.PasswordGenerationOptions{
					Symbols:   tt.symbols,
					MinLength: 20,
					MaxLength: 20,
				}

				for i := 0; i < 100; i++ {
					p, err := generatePass("", secretOptions)
					require.NoError(t, err)

					hasSymbol := false
					for _, sym := range tt.symbols {
						if strings.ContainsRune(string(p), sym) {
							hasSymbol = true
							break
						}
					}
					assert.True(t, hasSymbol, "password must contain at least one configured symbol")
				}
			})
		}
	})

	t.Run("proxyadmin constraints", func(t *testing.T) {
		secretOptions := &api.PasswordGenerationOptions{
			Symbols:   "!#$%&()*+,-.<=>?@[]^_{}~",
			MaxLength: 20,
			MinLength: 20,
		}

		for i := 0; i < 100; i++ {
			p, err := generatePass(users.ProxyAdmin, secretOptions)
			require.NoError(t, err)

			password := string(p)

			assert.NotContains(t, password, ":", "proxyadmin password must not contain ':'")
			assert.NotContains(t, password, ";", "proxyadmin password must not contain ';'")

			assert.False(t, strings.HasPrefix(password, "*"), "proxyadmin password must not start with '*'")
		}
	})

	t.Run("proxyadmin first character constraint with deterministic reader", func(t *testing.T) {
		secretOptions := &api.PasswordGenerationOptions{
			Symbols:   "!#$%&()*+,-.<=>?@[]^_{}~",
			MaxLength: 20,
			MinLength: 20,
		}
		idx := strings.Index(passSymbols(secretOptions), "*")
		require.NotEqual(t, -1, idx, "we can delete this test if passSymbols doesn't contain '*'")
		randReader = &repeatingReader{
			pattern: []byte{
				byte(idx),
				1,
				2,
				3,
				4,
				5,
				6,
				7,
				8,
			},
		}
		t.Cleanup(func() {
			randReader = rand.Reader
		})

		p, err := generatePass("", secretOptions)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(p), "*"), "expected '*' prefix when no rules are applied to the password")

		p, err = generatePass(users.ProxyAdmin, secretOptions)
		require.NoError(t, err)
		assert.False(t, strings.HasPrefix(string(p), "*"), "unexpected '*' prefix: proxyadmin passwords should not include it")
	})

	t.Run("proxyadmin with symbols guarantee", func(t *testing.T) {
		secretOptions := &api.PasswordGenerationOptions{
			Symbols:   "!@#$%",
			MinLength: 20,
			MaxLength: 20,
		}

		for i := 0; i < 100; i++ {
			p, err := generatePass(users.ProxyAdmin, secretOptions)
			require.NoError(t, err)

			password := string(p)

			allowedSymbols := "!@#$%"
			hasSymbol := false
			for _, sym := range allowedSymbols {
				if strings.ContainsRune(password, sym) {
					hasSymbol = true
					break
				}
			}
			assert.True(t, hasSymbol, "proxyadmin password must contain at least one allowed symbol")
		}
	})

	t.Run("no symbols configured", func(t *testing.T) {
		secretOptions := &api.PasswordGenerationOptions{
			Symbols:   "",
			MinLength: 20,
			MaxLength: 20,
		}

		for i := 0; i < 50; i++ {
			p, err := generatePass("", secretOptions)
			require.NoError(t, err)

			password := string(p)

			for _, char := range password {
				assert.Contains(t, passBaseSymbols, string(char),
					"password should only contain base symbols when no symbols configured")
			}
		}
	})

	t.Run("password randomness", func(t *testing.T) {
		secretOptions := &api.PasswordGenerationOptions{
			Symbols:   "!@#",
			MinLength: 20,
			MaxLength: 20,
		}

		passwords := make(map[string]bool)
		iterations := 100

		for i := 0; i < iterations; i++ {
			p, err := generatePass("", secretOptions)
			require.NoError(t, err)
			passwords[string(p)] = true
		}

		assert.Equal(t, iterations, len(passwords), "all generated passwords should be unique")
	})

	t.Run("variable length randomness", func(t *testing.T) {
		secretOptions := &api.PasswordGenerationOptions{
			Symbols:   "",
			MinLength: 10,
			MaxLength: 30,
		}

		lengths := make(map[int]int)

		for i := 0; i < 500; i++ {
			p, err := generatePass("", secretOptions)
			require.NoError(t, err)
			lengths[len(p)]++
		}

		assert.Greater(t, len(lengths), 1, "should generate passwords with varying lengths")

		for length := range lengths {
			assert.GreaterOrEqual(t, length, 10)
			assert.LessOrEqual(t, length, 30)
		}
	})
}
