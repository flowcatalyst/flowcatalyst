<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminPrincipalsUser extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/api/admin/principals/users';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUserBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUserConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUserBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUserConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}