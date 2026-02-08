<?php

namespace FlowCatalyst\Generated\Endpoint;

class UpdateIdentityProvider extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/identity-providers/{id}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\UpdateIdentityProviderNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateIdentityProviderBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\IdentityProviderDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\IdentityProviderDto', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\UpdateIdentityProviderNotFoundException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\UpdateIdentityProviderBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}