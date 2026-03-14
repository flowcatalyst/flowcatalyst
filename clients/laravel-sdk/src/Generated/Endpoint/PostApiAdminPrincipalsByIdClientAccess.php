<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminPrincipalsByIdClientAccess extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/principals/{id}/client-access');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse404', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}